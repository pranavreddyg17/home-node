package backup

import (
	"github.com/pranavreddyg17/home-node/internal/state"
	"strings"
	"testing"
)

func TestCleanupIsStrictAndSeparateFromExecution(t *testing.T) {
	cleanup := Cleanup{Version: 3, JobID: state.Random(), DeviceID: state.Random(), ManagementToken: state.Random(), RuntimeToken: state.Random()}
	raw, err := EncodeCleanup(cleanup)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := DecodeCleanup(raw); err != nil || got != cleanup {
		t.Fatal(got, err)
	}
	if _, err := DecodeLaunch(raw); err == nil {
		t.Fatal("cleanup accepted as launch")
	}
	if _, err := DecodeDispatch(raw); err == nil {
		t.Fatal("cleanup accepted as publication dispatch")
	}
	for _, invalid := range []string{strings.Replace(string(raw), `"version":3`, `"version":2`, 1), strings.Replace(string(raw), `"version":3`, `"version":3,"vers\u0069on":3`, 1), strings.Replace(string(raw), `"version":3`, `"Version":3`, 1), strings.TrimSuffix(string(raw), "}") + `,"command":"shell"}`, strings.Replace(string(raw), `"runtimeToken":"`+cleanup.RuntimeToken+`"`, `"runtimeToken":null`, 1), string(raw) + "{}", strings.Repeat(" ", MaxDispatchBytes+1)} {
		if got, err := DecodeCleanup([]byte(invalid)); err == nil || got != (Cleanup{}) {
			t.Fatal("unsafe cleanup accepted")
		}
	}
	if string(cleanupCompletionPacket(cleanup)) == string(launchCompletionPacket(Launch{JobID: cleanup.JobID})) {
		t.Fatal("completion domains collide")
	}
}
