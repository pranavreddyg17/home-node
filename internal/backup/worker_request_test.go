package backup

import (
	"github.com/pranavreddyg17/home-node/internal/state"
	"testing"
)

func TestWorkerRequestAcceptsOnlyLaunchAndCleanup(t *testing.T) {
	launch := Launch{Version: 2, JobID: state.Random(), DeviceID: state.Random(), ManagementToken: state.Random(), Release: "0.1.0", CatalogVersion: 1}
	cleanup := Cleanup{Version: 3, JobID: launch.JobID, DeviceID: launch.DeviceID, ManagementToken: launch.ManagementToken, RuntimeToken: state.Random()}
	launched, _ := EncodeLaunch(launch)
	cleaned, _ := EncodeCleanup(cleanup)
	if got, err := DecodeWorkerRequest(launched); err != nil || got.Launch == nil || *got.Launch != launch || got.Cleanup != nil {
		t.Fatal("launch route differs", err)
	}
	if got, err := DecodeWorkerRequest(cleaned); err != nil || got.Cleanup == nil || *got.Cleanup != cleanup || got.Launch != nil {
		t.Fatal("cleanup route differs", err)
	}
	dispatch, _ := launch.AcquiredDispatch(cleanup.RuntimeToken)
	raw, _ := EncodeDispatch(dispatch)
	for _, invalid := range [][]byte{raw, []byte(`{"version":2,"version":3}`), append(launched, cleaned...)} {
		if got, err := DecodeWorkerRequest(invalid); err == nil || got.Launch != nil || got.Cleanup != nil {
			t.Fatal("foreign worker schema accepted")
		}
	}
}
