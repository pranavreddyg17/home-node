package supervisor

import (
	"context"
	"strings"
	"unicode/utf8"
)

// validateGuestUIDAutomaticAllocation excludes the configured shadow automatic
// ranges and systemd v255's documented default container allocation range. It
// does not constrain explicit administrative IDs, other allocators, overridden
// command-line defaults, or a distro with different systemd build boundaries.
func validateGuestUIDAutomaticAllocation(ctx context.Context, pool GuestUIDPool, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if pool.validate() != nil || len(data) == 0 || len(data) > 1<<20 || !utf8.Valid(data) || strings.ContainsAny(string(data), "\r\x00") {
		return ErrPolicy
	}
	if pool.First <= 1879048191 && pool.Last >= 524288 {
		return ErrPolicy
	}
	required := map[string]uint32{"UID_MIN": 0, "UID_MAX": 0, "SYS_UID_MIN": 0, "SYS_UID_MAX": 0, "SUB_UID_MIN": 0, "SUB_UID_MAX": 0}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		if err := ctx.Err(); err != nil {
			return err
		}
		line, _, _ = strings.Cut(line, "#")
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if _, relevant := required[fields[0]]; !relevant {
			continue
		}
		if len(fields) != 2 || seen[fields[0]] {
			return ErrPolicy
		}
		value, err := decimalUID(fields[1])
		if err != nil {
			return err
		}
		required[fields[0]] = value
		seen[fields[0]] = true
	}
	for _, prefix := range []string{"UID", "SYS_UID", "SUB_UID"} {
		min, max := prefix+"_MIN", prefix+"_MAX"
		first, last := required[min], required[max]
		if !seen[min] || !seen[max] || first == 0 || last < first || pool.First <= last && pool.Last >= first {
			return ErrPolicy
		}
	}
	return ctx.Err()
}

// ObserveGuestUIDAutomaticAllocation reads protected host defaults only. This is
// an eligibility snapshot; exclusivity and future drift enforcement are separate.
func ObserveGuestUIDAutomaticAllocation(ctx context.Context, pool GuestUIDPool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if pool.validate() != nil {
		return ErrPolicy
	}
	return observeProtectedGuestIdentityConfig(ctx, "/etc", "login.defs", func(data []byte) error { return validateGuestUIDAutomaticAllocation(ctx, pool, data) })
}
