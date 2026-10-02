package updates

import "strings"

// ValidateInspectionSyscallFilter compares the expanded manager denyset with
// an independently qualified complete native-ABI profile. required must never
// be learned from this snapshot. An absent/unqualified profile cannot pass.
// This validates configuration, not actual seccomp installation or execution.
func ValidateInspectionSyscallFilter(properties []byte, required []string) error {
	if len(properties) == 0 || len(properties) > 2048 || len(required) == 0 || len(required) > 256 {
		return ErrInspectionResult
	}
	expected := map[string]bool{}
	for _, name := range required {
		if !inspectionSyscallName(name) || expected[name] {
			return ErrInspectionResult
		}
		expected[name] = true
	}
	line := strings.TrimSuffix(string(properties), "\n")
	value, ok := strings.CutPrefix(line, "SystemCallFilter=~")
	if !ok || strings.ContainsAny(value, "\r\n\x00") {
		return ErrInspectionResult
	}
	seen := map[string]bool{}
	for _, name := range strings.Fields(value) {
		if !inspectionSyscallName(name) || !expected[name] || seen[name] {
			return ErrInspectionResult
		}
		seen[name] = true
	}
	if len(seen) != len(expected) {
		return ErrInspectionResult
	}
	return nil
}

func inspectionSyscallName(name string) bool {
	if len(name) == 0 || len(name) > 64 {
		return false
	}
	for i := range len(name) {
		c := name[i]
		if !(c >= 'a' && c <= 'z' || c == '_' || i > 0 && c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}
