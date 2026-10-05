package control

import (
	"context"
	"errors"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/backup"
	"github.com/pranavreddyg17/home-node/internal/identity"
	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestPreviewAuthorityBindsSelectionAndCannotBecomeListing(t *testing.T) {
	s := testServer(t)
	token := seedBackupSession(t, s)
	if _, err := s.Store.DB.Exec("UPDATE identity SET claimed=1"); err != nil {
		t.Fatal(err)
	}
	actor, err := s.Identity.Authenticate(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	selected := state.Hash("selected")
	request, release, err := s.beginSnapshotPreviewRequest(context.Background(), actor, selected)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if err := s.verifySnapshotPreviewRequest(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	foreign := request
	foreign.SnapshotID = state.Hash("foreign")
	if err := s.verifySnapshotPreviewRequest(context.Background(), foreign); !errors.Is(err, identity.ErrDenied) {
		t.Fatal("selection not bound", err)
	}
	listing := backup.SnapshotPageRequest{Version: 4, Kind: "snapshot-page", RequestID: request.RequestID, DeviceID: request.DeviceID, Cursor: selected}
	if err := s.verifySnapshotRequest(context.Background(), listing); !errors.Is(err, identity.ErrDenied) {
		t.Fatal("preview became listing", err)
	}
	operation, err := s.snapshotPreviewOperationContext(request)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CloseBackupWork(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(operation.Err(), context.Canceled) {
		t.Fatal("preview dispatch survived shutdown")
	}
	if err := s.verifySnapshotPreviewRequest(context.Background(), request); !errors.Is(err, identity.ErrDenied) {
		t.Fatal("preview authority survived shutdown", err)
	}
	if _, release, err := s.beginSnapshotPreviewRequest(context.Background(), actor, selected); !errors.Is(err, identity.ErrDenied) || release != nil {
		t.Fatal("preview admitted after shutdown", err)
	}
}
