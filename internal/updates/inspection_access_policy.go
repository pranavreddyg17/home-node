package updates

import (
	"net/netip"
	"strings"
)

// ValidateInspectionAccessPolicy requires deny-all IP rules with no allow
// exceptions and the exact protected host paths. Live namespace/network/path
// enforcement and syscall filtering still need independent qualification.
func ValidateInspectionAccessPolicy(properties []byte) error {
	if len(properties) == 0 || len(properties) > 2048 || strings.ContainsAny(string(properties), "\r\x00") {
		return ErrInspectionResult
	}
	values := map[string]string{}
	for _, line := range strings.Split(strings.TrimSuffix(string(properties), "\n"), "\n") {
		name, value, ok := strings.Cut(line, "=")
		if !ok || name != "IPAddressDeny" && name != "IPAddressAllow" && name != "InaccessiblePaths" {
			return ErrInspectionResult
		}
		if _, exists := values[name]; exists {
			return ErrInspectionResult
		}
		values[name] = value
	}
	if len(values) != 3 || values["IPAddressAllow"] != "" {
		return ErrInspectionResult
	}
	prefixes := map[netip.Prefix]bool{}
	for _, value := range strings.Fields(values["IPAddressDeny"]) {
		prefix, err := netip.ParsePrefix(value)
		if err != nil || prefix.Bits() != 0 || prefix != prefix.Masked() || prefixes[prefix] {
			return ErrInspectionResult
		}
		prefixes[prefix] = true
	}
	if len(prefixes) != 2 || !prefixes[netip.MustParsePrefix("0.0.0.0/0")] || !prefixes[netip.MustParsePrefix("::/0")] {
		return ErrInspectionResult
	}
	expected := map[string]bool{"/etc/homenode": true, "/var/lib/homenode": true, "/var/lib/homenode-update": true, "/var/lib/homenode-backup": true, "/run/homenode": true, "/run/homenode-transfer": true}
	seen := map[string]bool{}
	for _, value := range strings.Fields(values["InaccessiblePaths"]) {
		path := strings.TrimPrefix(value, "-")
		if !expected[path] || seen[path] {
			return ErrInspectionResult
		}
		seen[path] = true
	}
	if len(seen) != len(expected) {
		return ErrInspectionResult
	}
	return nil
}
