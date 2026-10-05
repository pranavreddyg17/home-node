package main

import (
	"errors"
	"path/filepath"

	"github.com/pranavreddyg17/home-node/internal/backup"
	"github.com/pranavreddyg17/home-node/internal/control"
	"github.com/pranavreddyg17/home-node/internal/runtimeclient"
)

func backupExecutionConfiguration(repository, release string, catalog int64, socket string, controllerGID int, available bool, otherSockets ...string) (*control.BackupExecutionConfig, error) {
	if release == "" && catalog == 0 {
		return nil, nil
	}
	if !available || repository == "" || controllerGID < 100 || controllerGID > 999 || !filepath.IsAbs(socket) || filepath.Clean(socket) != socket {
		return nil, errors.New("invalid installed backup execution configuration")
	}
	for _, other := range otherSockets {
		if socket == other {
			return nil, errors.New("backup credential socket aliases another channel")
		}
	}
	sentinel := "backup-validation-identity"
	if _, err := backup.EncodeLaunch(backup.Launch{Version: 2, JobID: sentinel, DeviceID: sentinel, ManagementToken: sentinel, Release: release, CatalogVersion: catalog}); err != nil {
		return nil, err
	}
	dispatcher := runtimeclient.ActivatedBackupDispatcher{Socket: socket, ControllerGID: uint32(controllerGID)}
	return &control.BackupExecutionConfig{Release: release, CatalogVersion: catalog, Launch: dispatcher.DeliverLaunch, Cleanup: dispatcher.DeliverCleanup, SnapshotPage: dispatcher.SnapshotPage}, nil
}
