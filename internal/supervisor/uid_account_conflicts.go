package supervisor

import (
	"context"
	"strconv"
	"strings"
	"unicode/utf8"
)

// accountGuestUIDConflicts parses local account observations only. NSS providers,
// running processes and exclusive host-pool provisioning require separate checks.
func accountGuestUIDConflicts(ctx context.Context, pool GuestUIDPool, passwd, subuid []byte) (GuestUIDPool, error) {
	if err := ctx.Err(); err != nil {
		return GuestUIDPool{}, err
	}
	if pool.validate() != nil || len(passwd) == 0 || len(passwd) > 1<<20 || len(subuid) > 1<<20 || !utf8.Valid(passwd) || !utf8.Valid(subuid) {
		return GuestUIDPool{}, ErrPolicy
	}
	blocked := make(map[uint32]bool, len(pool.Blocked))
	for uid, value := range pool.Blocked {
		if value {
			blocked[uid] = true
		}
	}
	for _, line := range strings.Split(strings.TrimSuffix(string(passwd), "\n"), "\n") {
		if err := ctx.Err(); err != nil {
			return GuestUIDPool{}, err
		}
		fields := strings.Split(line, ":")
		if len(fields) != 7 || fields[0] == "" || strings.ContainsAny(line, "\x00\r") {
			return GuestUIDPool{}, ErrPolicy
		}
		uid, err := decimalUID(fields[2])
		if err != nil {
			return GuestUIDPool{}, err
		}
		if _, err = decimalUID(fields[3]); err != nil {
			return GuestUIDPool{}, err
		}
		if uid >= pool.First && uid <= pool.Last {
			blocked[uid] = true
		}
	}
	if len(subuid) > 0 {
		for _, line := range strings.Split(strings.TrimSuffix(string(subuid), "\n"), "\n") {
			if err := ctx.Err(); err != nil {
				return GuestUIDPool{}, err
			}
			fields := strings.Split(line, ":")
			if len(fields) != 3 || fields[0] == "" || strings.ContainsAny(line, "\x00\r") {
				return GuestUIDPool{}, ErrPolicy
			}
			start, err := decimalUID(fields[1])
			if err != nil {
				return GuestUIDPool{}, err
			}
			count, err := decimalUID(fields[2])
			if err != nil || count == 0 || uint64(start)+uint64(count) > 1<<32 {
				return GuestUIDPool{}, ErrPolicy
			}
			first, last := uint64(start), uint64(start)+uint64(count)-1
			if first < uint64(pool.First) {
				first = uint64(pool.First)
			}
			if last > uint64(pool.Last) {
				last = uint64(pool.Last)
			}
			for uid := first; uid <= last; uid++ {
				blocked[uint32(uid)] = true
			}
		}
	}
	pool.Blocked = blocked
	return pool, nil
}

func decimalUID(value string) (uint32, error) {
	if value == "" {
		return 0, ErrPolicy
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0, ErrPolicy
		}
	}
	n, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		return 0, ErrPolicy
	}
	return uint32(n), nil
}
