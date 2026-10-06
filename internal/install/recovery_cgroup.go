package install

import (
	"strings"
	"unicode/utf8"
)

func validateRecoveryEmptyCgroup(data []byte) error {
	if len(data) == 0 || len(data) > 1024 || !utf8.Valid(data) {
		return ErrConflict
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		key, value, ok := strings.Cut(line, " ")
		if !ok || (key != "populated" && key != "frozen") || seen[key] || value != "0" {
			return ErrConflict
		}
		seen[key] = true
	}
	if !seen["populated"] || !seen["frozen"] {
		return ErrConflict
	}
	return nil
}
