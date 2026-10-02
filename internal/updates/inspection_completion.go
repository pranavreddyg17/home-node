package updates

import (
	"strconv"
	"strings"
)

// ValidateInspectionCompletion checks the fixed systemd properties fetched by
// the privileged coordinator from the local manager for this invocation. These
// bytes must never originate in worker output, browser input or stored claims.
// A matching result document alone is insufficient execution evidence.
func ValidateInspectionCompletion(properties []byte, invocation string, notBeforeMicros uint64) error {
	if !validInspectionInvocation(invocation) || notBeforeMicros == 0 || len(properties) == 0 || len(properties) > 2048 {
		return ErrInspectionResult
	}
	expected := map[string]string{"InvocationID": invocation, "Result": "success", "ExecMainCode": "1", "ExecMainStatus": "0", "ActiveState": "active", "SubState": "exited"}
	seen, err := inspectionManagerProperties(properties)
	if err != nil {
		return err
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

func validInspectionInvocation(invocation string) bool {
	if len(invocation) != 32 {
		return false
	}
	nonzero := false
	for i := range len(invocation) {
		character := invocation[i]
		if !(character >= '0' && character <= '9' || character >= 'a' && character <= 'f') {
			return false
		}
		nonzero = nonzero || character != '0'
	}
	return nonzero
}

func inspectionManagerProperties(properties []byte) (map[string]string, error) {
	if len(properties) == 0 || len(properties) > 2048 {
		return nil, ErrInspectionResult
	}
	allowed := map[string]bool{"InvocationID": true, "Result": true, "ExecMainCode": true, "ExecMainStatus": true, "ActiveState": true, "SubState": true, "ExecMainStartTimestampMonotonic": true, "ExecMainExitTimestampMonotonic": true}
	seen := map[string]string{}
	for _, line := range strings.Split(strings.TrimSuffix(string(properties), "\n"), "\n") {
		name, value, ok := strings.Cut(line, "=")
		if !ok || !allowed[name] || strings.ContainsAny(value, "\r\x00") {
			return nil, ErrInspectionResult
		}
		if _, exists := seen[name]; exists {
			return nil, ErrInspectionResult
		}
		seen[name] = value
	}
	if len(seen) != len(allowed) {
		return nil, ErrInspectionResult
	}

	return seen, nil
}

// InspectionInvocationFromManager admits the fresh manager invocation while a
// oneshot is running or after successful completion. The coordinator must fetch
// these properties itself and retain the returned identity for result queries.
func InspectionInvocationFromManager(properties []byte, notBeforeMicros uint64) (string, error) {
	if notBeforeMicros == 0 {
		return "", ErrInspectionResult
	}
	values, err := inspectionManagerProperties(properties)
	if err != nil {
		return "", err
	}
	invocation := values["InvocationID"]
	if !validInspectionInvocation(invocation) {
		return "", ErrInspectionResult
	}
	if values["ActiveState"] == "active" && values["SubState"] == "exited" {
		if err := ValidateInspectionCompletion(properties, invocation, notBeforeMicros); err != nil {
			return "", err
		}
		return invocation, nil
	}
	if values["ActiveState"] != "activating" || values["SubState"] != "start" || values["Result"] != "success" || values["ExecMainCode"] != "0" || values["ExecMainStatus"] != "0" || values["ExecMainExitTimestampMonotonic"] != "0" {
		return "", ErrInspectionResult
	}
	start, err := strconv.ParseUint(values["ExecMainStartTimestampMonotonic"], 10, 64)
	if err != nil || strconv.FormatUint(start, 10) != values["ExecMainStartTimestampMonotonic"] || start < notBeforeMicros {
		return "", ErrInspectionResult
	}
	return invocation, nil
}
