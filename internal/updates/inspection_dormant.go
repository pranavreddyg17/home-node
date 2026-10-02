package updates

import "strings"

// ValidateInspectionDormant refuses a retained completed, running, failed or
// unloaded unit before a new launch. It is only a state observation: concurrent
// manager jobs and activation still require protected coordinator admission.
func ValidateInspectionDormant(properties []byte) error {
	if len(properties) == 0 || len(properties) > 2048 {
		return ErrInspectionResult
	}
	expected := map[string]string{"LoadState": "loaded", "ActiveState": "inactive", "SubState": "dead"}
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSuffix(string(properties), "\n"), "\n") {
		name, value, ok := strings.Cut(line, "=")
		required, allowed := expected[name]
		if !ok || !allowed || seen[name] || value != required {
			return ErrInspectionResult
		}
		seen[name] = true
	}
	if len(seen) != len(expected) {
		return ErrInspectionResult
	}
	return nil
}
