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

func TestPreliminaryBackupDeliveryRetainsCredentialOnRefusal(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "credential")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	launch := backup.Launch{Version: 2, JobID: strings.Repeat("a", 32), DeviceID: strings.Repeat("b", 32), ManagementToken: strings.Repeat("c", 32), Release: "0.1.0", CatalogVersion: 1}
	cleanup := backup.Cleanup{Version: 3, JobID: launch.JobID, DeviceID: launch.DeviceID, ManagementToken: launch.ManagementToken, RuntimeToken: strings.Repeat("d", 32)}
	for _, dispatcher := range []ActivatedBackupDispatcher{{Socket: "relative", ControllerGID: 351}, {Socket: "/run/../run/credential.sock", ControllerGID: 351}, {Socket: "/run/credential.sock", ControllerGID: 0}, {Socket: "/run/credential.sock", ControllerGID: 1000}} {
		if err := dispatcher.DeliverLaunch(context.Background(), launch, file); err == nil {
			t.Fatal("unsafe launch configuration admitted")
		}
		if err := dispatcher.DeliverCleanup(context.Background(), cleanup, file); err == nil {
			t.Fatal("unsafe cleanup configuration admitted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dispatcher := ActivatedBackupDispatcher{Socket: "/run/homenode-backup/credential.sock", ControllerGID: 351}
	if err := dispatcher.DeliverLaunch(ctx, launch, file); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := dispatcher.DeliverCleanup(ctx, cleanup, file); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	launch.Version = 1
	cleanup.Version = 2
	if err := dispatcher.DeliverLaunch(context.Background(), launch, file); err == nil {
		t.Fatal("wrong launch domain admitted")
	}
	if err := dispatcher.DeliverCleanup(context.Background(), cleanup, file); err == nil {
		t.Fatal("wrong cleanup domain admitted")
	}
	if _, err := file.Stat(); err != nil {
		t.Fatal("caller credential closed", err)
	}
}
