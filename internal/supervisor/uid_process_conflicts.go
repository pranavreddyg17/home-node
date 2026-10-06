package supervisor

import (
	"context"
	"strings"
	"unicode/utf8"
)

// processGuestUIDConflicts parses one bounded kernel process-status observation.
// Real, effective, saved and filesystem UIDs all reserve host authority. It
// cannot establish process absence or exclude future credential changes.
func processGuestUIDConflicts(ctx context.Context, pool GuestUIDPool, status []byte) (GuestUIDPool, error) {
	if err := ctx.Err(); err != nil {
		return GuestUIDPool{}, err
	}
	if pool.validate() != nil || len(status) == 0 || len(status) > 64<<10 || !utf8.Valid(status) || strings.ContainsAny(string(status), "\r\x00") {
		return GuestUIDPool{}, ErrPolicy
	}
	var credentials [4]uint32
	found := false
	for _, line := range strings.Split(string(status), "\n") {
		if err := ctx.Err(); err != nil {
			return GuestUIDPool{}, err
		}
		if !strings.HasPrefix(line, "Uid:") {
			continue
		}
		if found {
			return GuestUIDPool{}, ErrPolicy
		}
		fields := strings.Fields(strings.TrimPrefix(line, "Uid:"))
		if len(fields) != 4 {
			return GuestUIDPool{}, ErrPolicy
		}
		for i, field := range fields {
			uid, err := decimalUID(field)
			if err != nil {
				return GuestUIDPool{}, err
			}
			credentials[i] = uid
		}
		found = true
	}
	if !found {
		return GuestUIDPool{}, ErrPolicy
	}
	blocked := make(map[uint32]bool, len(pool.Blocked)+4)
	for uid, value := range pool.Blocked {
		if value {
			blocked[uid] = true
		}
	}
	for _, uid := range credentials {
		if uid >= pool.First && uid <= pool.Last {
			blocked[uid] = true
		}
	}
	pool.Blocked = blocked
	return pool, nil
}
