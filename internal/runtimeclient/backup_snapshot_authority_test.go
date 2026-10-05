package runtimeclient

import (
	"context"
	"errors"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/backup"
	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestSnapshotAuthorityRequiresConfiguredTransportAndLiveContext(t *testing.T) {
	request := backup.SnapshotPageRequest{Version: 4, Kind: "snapshot-page", RequestID: state.Random(), DeviceID: state.Random()}
	for _, socket := range []string{"", "relative", "/run/../run/management.sock"} {
		client := NewSnapshotPageAuthority(socket, 0)
		if err := client.VerifySnapshotPage(context.Background(), request); err == nil {
			t.Fatal("unsafe socket admitted", socket)
		}
		client.Close()
	}
	client := NewSnapshotPageAuthority("/run/homenode/maintenance.sock", 0)
	defer client.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := client.VerifySnapshotPage(ctx, request); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled authority attempted", err)
	}
	request.Kind = "cleanup"
	if err := client.VerifySnapshotPage(context.Background(), request); err == nil {
		t.Fatal("foreign request domain admitted")
	}
}
