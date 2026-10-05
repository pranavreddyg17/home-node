package runtimeclient

import (
	"context"
	"errors"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/backup"
	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestSnapshotPreviewAuthorityRequiresConfiguredTransportAndLiveContext(t *testing.T) {
	request := backup.SnapshotPreviewRequest{Version: 5, Kind: "snapshot-preview", RequestID: state.Random(), DeviceID: state.Random(), SnapshotID: state.Hash("selected")}
	for _, socket := range []string{"", "relative", "/run/../run/management.sock"} {
		client := NewSnapshotPreviewAuthority(socket, 0)
		if err := client.VerifySnapshotPreview(context.Background(), request); err == nil {
			t.Fatal("unsafe socket admitted", socket)
		}
		client.Close()
	}
	client := NewSnapshotPreviewAuthority("/run/homenode/maintenance.sock", 0)
	defer client.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := client.VerifySnapshotPreview(ctx, request); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled authority attempted", err)
	}
	request.Kind = "cleanup"
	if err := client.VerifySnapshotPreview(context.Background(), request); err == nil {
		t.Fatal("foreign request domain admitted")
	}
}
