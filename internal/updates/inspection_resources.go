package updates

import "strings"

// ValidateInspectionResources checks manager-configured execution limits. Live
// cgroup enforcement must also be qualified; configured values alone do not
// prove that the kernel controllers enforce the workload boundary.
func ValidateInspectionResources(properties []byte) error {
	expected := map[string]string{"MemoryMax": "268435456", "MemorySwapMax": "0", "CPUQuotaPerSecUSec": "500ms", "TasksMax": "32", "OOMPolicy": "kill", "KillMode": "control-group", "Restart": "no", "TimeoutStartUSec": "2min 30s", "TimeoutStopUSec": "5s"}
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
