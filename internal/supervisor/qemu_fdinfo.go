package supervisor

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// Only parses independently authenticated procfs bytes; it grants no authority.
func parseQEMUFDFlags(data []byte) (uint32, error) {
	if len(data) == 0 || len(data) > 8192 || !utf8.Valid(data) || strings.ContainsAny(string(data), "\r\x00") {
		return 0, ErrPolicy
	}
	found := false
	var flags uint32
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "flags:") {
			continue
		}
		if found {
			return 0, ErrPolicy
		}
		found = true
		value := strings.TrimSpace(strings.TrimPrefix(line, "flags:"))
		if value == "" || strings.Trim(value, "01234567") != "" {
			return 0, ErrPolicy
		}
		n, err := strconv.ParseUint(value, 8, 32)
		if err != nil {
			return 0, ErrPolicy
		}
		flags = uint32(n)
	}
	if !found {
		return 0, ErrPolicy
	}
	return flags, nil
}
