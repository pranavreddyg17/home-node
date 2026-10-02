package updates

import "strings"

// ValidateInspectionIsolation checks core manager-configured confinement.
// Network/syscall/path policies and live enforcement need additional checks;
// this snapshot alone is not complete activation authorization.
func ValidateInspectionIsolation(properties []byte) error {
	expected := map[string]string{"NoNewPrivileges": "yes", "CapabilityBoundingSet": "", "AmbientCapabilities": "", "ProtectSystem": "strict", "ProtectHome": "yes", "PrivateTmp": "yes", "PrivateDevices": "yes", "PrivateNetwork": "yes", "ProtectKernelTunables": "yes", "ProtectKernelModules": "yes", "ProtectKernelLogs": "yes", "ProtectControlGroups": "yes", "ProtectProc": "invisible", "ProcSubset": "pid", "RestrictSUIDSGID": "yes", "RestrictRealtime": "yes", "LockPersonality": "yes"}
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
