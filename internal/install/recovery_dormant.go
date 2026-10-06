package install

import (
	"strings"
	"unicode/utf8"
)

// validateRecoveryDormant parses a bounded system manager observation for one
// fixed service. Dormancy is not a retained exclusion lease: production handoff
// must prevent activation and reobserve before publishing any restored file.
func validateRecoveryDormant(data []byte) error {
	if len(data) == 0 || len(data) > 1024 || !utf8.Valid(data) {
		return ErrConflict
	}
	expected := map[string]string{"LoadState": "loaded", "ActiveState": "inactive", "SubState": "dead", "MainPID": "0", "ControlPID": "0"}
	seen := make(map[string]bool, len(expected))
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		key, value, ok := strings.Cut(line, "=")
		want, known := expected[key]
		if !ok || !known || seen[key] || value != want {
			return ErrConflict
		}
		seen[key] = true
	}
	if len(seen) != len(expected) {
		return ErrConflict
	}
	return nil
}
