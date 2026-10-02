package updates

import "strings"

// ValidateInspectionProcessPolicy checks the configured namespace, socket and
// syscall-architecture restrictions. Syscall denysets, IP/path policies and live
// enforcement remain separate activation prerequisites.
func ValidateInspectionProcessPolicy(properties []byte) error {
	expected := map[string]string{"RestrictNamespaces": "yes", "RestrictAddressFamilies": "AF_UNIX", "SystemCallArchitectures": "native"}
	if len(properties) == 0 || len(properties) > 2048 {
		return ErrInspectionResult
	}
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
