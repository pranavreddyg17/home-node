package runtimeclient

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/backup"
)

func TestSnapshotPageDeliveryRefusesUnqualifiedConfiguration(t *testing.T) {
	credential, err := os.CreateTemp(t.TempDir(), "credential")
	if err != nil {
		t.Fatal(err)
	}
	defer credential.Close()
	request := backup.SnapshotPageRequest{Version: 4, Kind: "snapshot-page", RequestID: strings.Repeat("a", 32), DeviceID: strings.Repeat("b", 32)}
	for _, dispatcher := range []ActivatedBackupDispatcher{
		{Socket: "relative", ControllerGID: 351},
		{Socket: "/run/../run/credential.sock", ControllerGID: 351},
		{Socket: "/run/credential.sock", ControllerGID: 0},
		{Socket: "/run/credential.sock", ControllerGID: 1000},
	} {
		page, err := dispatcher.SnapshotPage(context.Background(), request, credential)
		if err == nil || page.Snapshots != nil || page.Next != "" {
			t.Fatal("unqualified selector returned candidates", page, err)
		}
	}
	dispatcher := ActivatedBackupDispatcher{Socket: "/run/homenode-backup/credential.sock", ControllerGID: 351}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	page, err := dispatcher.SnapshotPage(ctx, request, credential)
	if !errors.Is(err, context.Canceled) || page.Snapshots != nil || page.Next != "" {
		t.Fatal("cancelled selector returned candidates", page, err)
	}
	request.Kind = "cleanup"
	page, err = dispatcher.SnapshotPage(context.Background(), request, credential)
	if err == nil || page.Snapshots != nil || page.Next != "" {
		t.Fatal("maintenance-domain selector admitted", page, err)
	}
	if _, err := credential.Stat(); err != nil {
		t.Fatal("selector closed caller credential", err)
	}
}
