//go:build linux

package backup

import (
	"context"
	"os"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/disktransport"
	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestSnapshotPageCredentialHandoff(t *testing.T) {
	uid := uint32(os.Geteuid())
	if uid == 0 {
		t.Skip("private controller must be unprivileged")
	}
	request := SnapshotPageRequest{Version: 4, Kind: "snapshot-page", RequestID: state.Random(), DeviceID: state.Random()}
	raw, err := EncodeSnapshotPageRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	credential, err := CreateRepositoryPassword([]byte("fixture-selector-secret"))
	if err != nil {
		t.Fatal(err)
	}
	defer credential.Close()
	for _, scenario := range []string{"valid", "foreign-peer", "maintenance-domain"} {
		t.Run(scenario, func(t *testing.T) {
			sender, receiver := dispatchPair(t)
			message, expected := raw, uid
			if scenario == "foreign-peer" {
				expected++
			}
			if scenario == "maintenance-domain" {
				message, err = EncodeLaunch(Launch{Version: 2, JobID: state.Random(), DeviceID: request.DeviceID, ManagementToken: state.Random(), Release: "0.1.0", CatalogVersion: 1})
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := disktransport.SendFile(context.Background(), sender, message, credential); err != nil {
				t.Fatal(err)
			}
			got, received, err := ReceiveCredentialSnapshotPage(context.Background(), receiver, expected)
			if scenario != "valid" {
				if err == nil || received != nil || got != (SnapshotPageRequest{}) {
					t.Fatal("invalid handoff admitted", err)
				}
				return
			}
			if err != nil || got != request || received == nil {
				t.Fatal("selector handoff lost", got, err)
			}
			defer received.Close()
			secret, err := ReadRepositoryPassword(context.Background(), received)
			if err != nil || string(secret) != "fixture-selector-secret" {
				t.Fatal("selector credential lost", err)
			}
			clear(secret)
		})
	}
	if _, err := credential.Stat(); err != nil {
		t.Fatal("receiver invalidated caller credential", err)
	}
	sender, _ := dispatchPair(t)
	if err := SendActivatedCredentialSnapshotPage(context.Background(), sender, request, credential); err == nil {
		t.Fatal("activated selector trusted unprivileged listener creator")
	}
}
