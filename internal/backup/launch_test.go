package backup

import (
	"bytes"
	"github.com/pranavreddyg17/home-node/internal/state"
	"strings"
	"testing"
)

func TestBackupLaunchIsDistinctAndCarriesNoRuntimeAuthority(t *testing.T) {
	launch := Launch{Version: 2, JobID: state.Random(), DeviceID: state.Random(), ManagementToken: state.Random(), Release: "0.1.0", CatalogVersion: 1}
	raw, err := EncodeLaunch(launch)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("runtimeToken")) {
		t.Fatal("launch retained root authority")
	}
	if got, err := DecodeLaunch(raw); err != nil || got != launch {
		t.Fatal(got, err)
	}
	if _, err := DecodeDispatch(raw); err == nil {
		t.Fatal("launch accepted as acquired dispatch")
	}
	root := state.Random()
	dispatch, err := launch.AcquiredDispatch(root)
	if err != nil || dispatch.RuntimeToken != root || dispatch.JobID != launch.JobID || dispatch.DeviceID != launch.DeviceID || dispatch.ManagementToken != launch.ManagementToken {
		t.Fatal("acquired identity differs", dispatch, err)
	}
	acquired, _ := EncodeDispatch(dispatch)
	if _, err := DecodeLaunch(acquired); err == nil {
		t.Fatal("dispatch accepted as preliminary launch")
	}
	for _, root := range []string{"", "short"} {
		if _, err := launch.AcquiredDispatch(root); err == nil {
			t.Fatal("invalid root accepted")
		}
	}
	for _, invalid := range []string{strings.Replace(string(raw), `"version":2`, `"version":1`, 1), strings.Replace(string(raw), `"version":2`, `"version":2,"vers\u0069on":2`, 1), strings.Replace(string(raw), `"version":2`, `"Version":2`, 1), strings.TrimSuffix(string(raw), "}") + `,"password":"secret"}`, strings.TrimSuffix(string(raw), "}") + `,"runtimeToken":"` + root + `"}`, strings.Replace(string(raw), `"version":2`, `"version":null`, 1), string(raw) + "{}", strings.Repeat(" ", MaxDispatchBytes+1)} {
		if got, err := DecodeLaunch([]byte(invalid)); err == nil || got != (Launch{}) {
			t.Fatal("unsafe launch accepted")
		}
	}
}
