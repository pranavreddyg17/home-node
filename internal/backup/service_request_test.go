package backup

import (
	"github.com/pranavreddyg17/home-node/internal/state"
	"testing"
)

func TestServiceRequestKeepsMetadataAndMaintenanceDisjoint(t *testing.T) {
	snapshot := SnapshotPageRequest{Version: 4, Kind: "snapshot-page", RequestID: state.Random(), DeviceID: state.Random()}
	raw, err := EncodeSnapshotPageRequest(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	request, err := decodeServiceRequest(raw)
	if err != nil || request.snapshot == nil || *request.snapshot != snapshot || request.maintenance != nil || request.page == nil {
		t.Fatal("snapshot domain lost", request, err)
	}
	if _, err := DecodeWorkerRequest(raw); err == nil {
		t.Fatal("metadata admitted by maintenance decoder")
	}
	launch := Launch{Version: 2, JobID: state.Random(), DeviceID: snapshot.DeviceID, ManagementToken: state.Random(), Release: "0.1.0", CatalogVersion: 1}
	raw, err = EncodeLaunch(launch)
	if err != nil {
		t.Fatal(err)
	}
	request, err = decodeServiceRequest(raw)
	if err != nil || request.maintenance == nil || request.maintenance.Launch == nil || *request.maintenance.Launch != launch || request.snapshot != nil || request.page != nil {
		t.Fatal("maintenance domain lost", request, err)
	}
	if _, err := decodeServiceRequest([]byte(`{"version":4}`)); err == nil {
		t.Fatal("incomplete request admitted")
	}
}
