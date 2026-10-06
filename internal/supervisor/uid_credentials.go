package supervisor

import (
	"strings"
	"unicode/utf8"
)

// validateGuestDACCredentials verifies one task's exact reserved identity and
// absence of capability/new-privilege authority. Its bytes must independently
// come from protected host procfs; parsing alone grants no runtime authority.
func validateGuestDACCredentials(status []byte, uid, gid uint32) error {
	if uid < 65536 || uid > 1<<31-1 || gid == 0 || gid > 1<<31-1 || len(status) == 0 || len(status) > 64<<10 || !utf8.Valid(status) || strings.ContainsAny(string(status), "\r\x00") {
		return ErrPolicy
	}
	expected := map[string]bool{"Uid": true, "Gid": true, "Groups": true, "CapInh": true, "CapPrm": true, "CapEff": true, "CapAmb": true, "NoNewPrivs": true}
	seen := make(map[string]bool, len(expected))
	for _, line := range strings.Split(string(status), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok || !expected[key] {
			continue
		}
		if seen[key] {
			return ErrPolicy
		}
		seen[key] = true
		value = strings.TrimSpace(value)
		switch key {
		case "Uid", "Gid":
			fields := strings.Fields(value)
			if len(fields) != 4 {
				return ErrPolicy
			}
			target := uid
			if key == "Gid" {
				target = gid
			}
			for _, field := range fields {
				actual, err := decimalUID(field)
				if err != nil || actual != target {
					return ErrPolicy
				}
			}
		case "Groups":
			fields := strings.Fields(value)
			if len(fields) > 1 {
				return ErrPolicy
			}
			if len(fields) == 1 {
				actual, err := decimalUID(fields[0])
				if err != nil || actual != gid {
					return ErrPolicy
				}
			}
		case "NoNewPrivs":
			if value != "1" {
				return ErrPolicy
			}
		default:
			if value != "0000000000000000" {
				return ErrPolicy
			}
		}
	}
	if len(seen) != len(expected) {
		return ErrPolicy
	}
	return nil
}
