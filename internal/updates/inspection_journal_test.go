package updates

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestInspectionJournalEnvelopeRequiresExactTrustedContext(t *testing.T) {
	identity := InspectionIdentity{OperationID: "inspection-fixture-000001", Release: "0.1.0", PackageSHA256: strings.Repeat("ab", 32), PackageLength: 12}
	invocation, boot := strings.Repeat("a", 32), strings.Repeat("b", 32)
	message, _ := json.Marshal(InspectionResult{Schema: 1, OperationID: identity.OperationID, Release: identity.Release, PackageSHA256: identity.PackageSHA256, PackageLength: 12, ContentValid: true})
	values := map[string]string{"MESSAGE": string(message), "_SYSTEMD_UNIT": "homenode-inspect.service", "_SYSTEMD_INVOCATION_ID": invocation, "_BOOT_ID": boot, "_TRANSPORT": "stdout"}
	valid, _ := json.Marshal(values)
	if _, err := ValidateInspectionJournalEntry(valid, identity, invocation, boot); err != nil {
		t.Fatal(err)
	}
	values["__SEQNUM"] = "123"
	values["__SEQNUM_ID"] = strings.Repeat("c", 32)
	withSequence, _ := json.Marshal(values)
	if _, err := ValidateInspectionJournalEntry(withSequence, identity, invocation, boot); err != nil {
		t.Fatal("native sequence metadata refused", err)
	}
	for _, sequence := range []string{"", "0", "0123", "+123", "-1", "18446744073709551616"} {
		values["__SEQNUM"] = sequence
		data, _ := json.Marshal(values)
		if _, err := ValidateInspectionJournalEntry(data, identity, invocation, boot); err == nil {
			t.Fatal("invalid native sequence admitted", sequence)
		}
	}
	values["__SEQNUM"] = "123"
	delete(values, "__SEQNUM_ID")
	incompleteSequence, _ := json.Marshal(values)
	if _, err := ValidateInspectionJournalEntry(incompleteSequence, identity, invocation, boot); err == nil {
		t.Fatal("incomplete sequence metadata admitted")
	}
	delete(values, "__SEQNUM")
	for key, replacement := range map[string]string{"_SYSTEMD_UNIT": "other.service", "_SYSTEMD_INVOCATION_ID": boot, "_BOOT_ID": invocation, "_TRANSPORT": "journal", "_LINE_BREAK": "line-max", "MESSAGE": "{}"} {
		changed := map[string]string{}
		for k, v := range values {
			changed[k] = v
		}
		changed[key] = replacement
		data, _ := json.Marshal(changed)
		if _, err := ValidateInspectionJournalEntry(data, identity, invocation, boot); err == nil {
			t.Fatal("mismatched journal envelope admitted", key)
		}
	}
	for _, data := range [][]byte{append(append([]byte(nil), valid...), valid...), []byte(`{"MESSAGE":[],"_TRANSPORT":"stdout"}`), []byte(strings.Repeat("x", 8193)), []byte(strings.Replace(string(valid), `"_TRANSPORT":"stdout"`, `"_TRANSPORT":"stdout","_TRANSPORT":"stdout"`, 1))} {
		if _, err := ValidateInspectionJournalEntry(data, identity, invocation, boot); err == nil {
			t.Fatal("ambiguous journal envelope admitted")
		}
	}
}
