package runtimeclient

import (
	"context"
	"errors"
	"github.com/pranavreddyg17/home-node/internal/backup"
	"os"
	"strings"
	"testing"
)

func TestBackupDispatchRejectsConfigurationAndCancellationBeforeDelivery(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "credential")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	job := backup.Dispatch{Version: 1, JobID: strings.Repeat("a", 32), DeviceID: strings.Repeat("b", 32), ManagementToken: strings.Repeat("c", 32), RuntimeToken: strings.Repeat("d", 32), Release: "0.1.0", CatalogVersion: 1}
	for _, dispatcher := range []ActivatedBackupDispatcher{{Socket: "relative", ControllerGID: 351}, {Socket: "/run/../run/credential.sock", ControllerGID: 351}, {Socket: "/run/credential.sock", ControllerGID: 0}, {Socket: "/run/credential.sock", ControllerGID: 1000}} {
		if err = dispatcher.Deliver(context.Background(), job, file); err == nil {
			t.Fatal("unsafe dispatch configuration admitted", dispatcher)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dispatcher := ActivatedBackupDispatcher{Socket: "/run/homenode-backup/credential.sock", ControllerGID: 351}
	if err = dispatcher.Deliver(ctx, job, file); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled delivery attempted", err)
	}
	if _, err = file.Stat(); err != nil {
		t.Fatal("caller credential closed", err)
	}
}
