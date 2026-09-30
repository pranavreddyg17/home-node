package catalog

import (
	"strconv"
	"strings"
)

// VersionFloor accepts only a canonical positive decimal version, optionally
// followed by one newline. Missing, malformed or zero trust floors fail closed.
func VersionFloor(data []byte) (int64, error) {
	if len(data) == 0 || len(data) > 20 {
		return 0, ErrUntrusted
	}
	value := strings.TrimSuffix(string(data), "\n")
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n < 1 || strconv.FormatInt(n, 10) != value {
		return 0, ErrUntrusted
	}
	return n, nil
}
