package supervisor

import (
	"strconv"
	"strings"
)

// A PID-file value is a lookup hint, never process authentication.
func parseDomainPID(data []byte) (int, error) {
	if len(data) == 0 || len(data) > 32 {
		return 0, ErrPolicy
	}
	value := strings.TrimSuffix(string(data), "\n")
	n, err := strconv.ParseUint(value, 10, 31)
	if err != nil || n == 0 || strconv.FormatUint(n, 10) != value {
		return 0, ErrPolicy
	}
	return int(n), nil
}
