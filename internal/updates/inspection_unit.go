package updates

import "strings"

// ValidateInspectionUnitIdentity checks a bounded local-manager snapshot of
// the fixed owned unit's load identity. This is only one activation prerequisite;
// effective isolation/resource properties must also be checked before start.
func ValidateInspectionUnitIdentity(properties []byte) error {
	if len(properties) == 0 || len(properties) > 2048 {
		return ErrInspectionResult
	}
	expected := map[string]string{"Id": "homenode-inspect.service", "LoadState": "loaded", "FragmentPath": "/etc/systemd/system/homenode-inspect.service", "DropInPaths": "", "NeedDaemonReload": "no", "Type": "oneshot", "RemainAfterExit": "yes", "DynamicUser": "yes", "Transient": "no"}
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
