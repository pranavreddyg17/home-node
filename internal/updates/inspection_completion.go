package updates

import (
	"encoding/hex"
	"strconv"
	"strings"
)

// ValidateInspectionCompletion checks the fixed systemd properties fetched by
// the privileged coordinator from the local manager for this invocation. These
// bytes must never originate in worker output, browser input or stored claims.
// A matching result document alone is insufficient execution evidence.
func ValidateInspectionCompletion(properties []byte, invocation string, notBeforeMicros uint64) error {
	decoded, err := hex.DecodeString(invocation)
	if err != nil || len(decoded) != 16 || hex.EncodeToString(decoded) != invocation || invocation == strings.Repeat("0", 32) || notBeforeMicros == 0 || len(properties) == 0 || len(properties) > 2048 {
		return ErrInspectionResult
	}
	expected := map[string]string{"InvocationID": invocation, "Result": "success", "ExecMainCode": "1", "ExecMainStatus": "0", "ActiveState": "inactive", "SubState": "dead"}
	allowed := map[string]bool{"InvocationID": true, "Result": true, "ExecMainCode": true, "ExecMainStatus": true, "ActiveState": true, "SubState": true, "ExecMainStartTimestampMonotonic": true, "ExecMainExitTimestampMonotonic": true}
	seen := map[string]string{}
	for _, line := range strings.Split(strings.TrimSuffix(string(properties), "\n"), "\n") {
		name, value, ok := strings.Cut(line, "=")
		if !ok || !allowed[name] || strings.ContainsAny(value, "\r\x00") {
			return ErrInspectionResult
		}
		if _, exists := seen[name]; exists {
			return ErrInspectionResult
		}
		seen[name] = value
	}
	if len(seen) != len(allowed) {
		return ErrInspectionResult
	}
	for name, value := range expected {
		if seen[name] != value {
			return ErrInspectionResult
		}
	}
	start, err := strconv.ParseUint(seen["ExecMainStartTimestampMonotonic"], 10, 64)
	if err != nil || strconv.FormatUint(start, 10) != seen["ExecMainStartTimestampMonotonic"] || start < notBeforeMicros {
		return ErrInspectionResult
	}
	end, err := strconv.ParseUint(seen["ExecMainExitTimestampMonotonic"], 10, 64)
	if err != nil || strconv.FormatUint(end, 10) != seen["ExecMainExitTimestampMonotonic"] || end < start {
		return ErrInspectionResult
	}
	return nil
}
