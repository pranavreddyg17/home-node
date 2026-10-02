package updates

import (
	"strings"
	"testing"
)

func TestInspectionDormantRefusesRetainedAndUnknownUnit(t *testing.T) {
	valid := "LoadState=loaded\nActiveState=inactive\nSubState=dead\n"
	if err := ValidateInspectionDormant([]byte(valid)); err != nil {
		t.Fatal(err)
	}
	for _, altered := range []string{
		strings.Replace(valid, "loaded", "not-found", 1),
		strings.Replace(valid, "inactive", "active", 1),
		strings.Replace(valid, "dead", "exited", 1),
		strings.Replace(valid, "inactive", "activating", 1),
		strings.Replace(valid, "inactive", "failed", 1),
		valid + "ActiveState=inactive\n", valid + "Job=0\n", "ActiveState=inactive\nSubState=dead\n",
		valid + "\n", strings.Replace(valid, "dead", "dead\r", 1), strings.Repeat("x", 2049),
	} {
		if err := ValidateInspectionDormant([]byte(altered)); err == nil {
			t.Fatal("unsafe dormant state admitted", altered)
		}
	}
}
