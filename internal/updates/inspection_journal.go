package updates

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"
)

// ValidateInspectionJournalEntry validates one bounded local-journal envelope.
// The coordinator must fetch these bytes itself from the protected local
// journal after recorded manager completion; supplied JSON is not authenticated
// merely because it names trusted journal fields. Missing/truncated output
// refuses, and content validity grants no installation authority.
func ValidateInspectionJournalEntry(data []byte, expected InspectionIdentity, invocation, bootHex string) (InspectionResult, error) {
	var zero InspectionResult
	if len(data) == 0 || len(data) > 8192 || !validInspectionInvocation(invocation) || !validInspectionInvocation(bootHex) || !validInspectionIdentity(expected) {
		return zero, ErrInspectionResult
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return zero, ErrInspectionResult
	}
	values := map[string]string{}
	allowed := map[string]bool{"MESSAGE": true, "_SYSTEMD_UNIT": true, "_SYSTEMD_INVOCATION_ID": true, "_BOOT_ID": true, "_TRANSPORT": true, "_LINE_BREAK": true, "__CURSOR": true, "__REALTIME_TIMESTAMP": true, "__MONOTONIC_TIMESTAMP": true, "__SEQNUM": true, "__SEQNUM_ID": true}
	for decoder.More() {
		token, err = decoder.Token()
		name, ok := token.(string)
		if err != nil || !ok || !allowed[name] {
			return zero, ErrInspectionResult
		}
		if _, duplicate := values[name]; duplicate {
			return zero, ErrInspectionResult
		}
		valueToken, valueErr := decoder.Token()
		value, isString := valueToken.(string)
		if valueErr != nil || !isString {
			return zero, ErrInspectionResult
		}
		values[name] = value
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') {
		return zero, ErrInspectionResult
	}
	if decoder.Decode(new(any)) != io.EOF {
		return zero, ErrInspectionResult
	}
	sequence, hasSequence := values["__SEQNUM"]
	sequenceID, hasSequenceID := values["__SEQNUM_ID"]
	if hasSequence || hasSequenceID {
		number, err := strconv.ParseUint(sequence, 10, 64)
		if !hasSequence || !hasSequenceID || err != nil || number == 0 || strconv.FormatUint(number, 10) != sequence || !validInspectionInvocation(sequenceID) {
			return zero, ErrInspectionResult
		}
	}
	if values["_SYSTEMD_UNIT"] != "homenode-inspect.service" || values["_SYSTEMD_INVOCATION_ID"] != invocation || values["_BOOT_ID"] != bootHex || values["_TRANSPORT"] != "stdout" || values["_LINE_BREAK"] != "" {
		return zero, ErrInspectionResult
	}
	return ValidateInspectionResult([]byte(values["MESSAGE"]), expected)
}
