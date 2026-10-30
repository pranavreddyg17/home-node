package supervisor

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// Protected procfs bytes are required independently. Mode/count establishes
// active filtering only, never the identity or safety of the installed BPF.
func validateQEMUSeccompStatus(status []byte) error {
	if len(status) == 0 || len(status) > 64<<10 || !utf8.Valid(status) || strings.ContainsAny(string(status), "\r\x00") {
		return ErrPolicy
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(status), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok || (key != "Seccomp" && key != "Seccomp_filters") {
			continue
		}
		if seen[key] {
			return ErrPolicy
		}
		seen[key] = true
		value = strings.TrimSpace(value)
		if key == "Seccomp" {
			if value != "2" {
				return ErrPolicy
			}
			continue
		}
		n, err := strconv.ParseUint(value, 10, 32)
		if err != nil || n == 0 || strconv.FormatUint(n, 10) != value {
			return ErrPolicy
		}
	}
	if len(seen) != 2 {
		return ErrPolicy
	}
	return nil
}
