package runtimeclient

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/backup"
	"github.com/pranavreddyg17/home-node/internal/state"
)

type previewAuthorityFixture struct {
	request backup.SnapshotPreviewRequest
	err     error
	calls   int
}

func (a *previewAuthorityFixture) VerifySnapshotPreview(_ context.Context, request backup.SnapshotPreviewRequest) error {
	a.calls++
	if request != a.request {
		return errors.New("foreign request")
	}
	return a.err
}

type previewReaderFixture struct {
	snapshot string
	policy   backup.RestorePolicy
	result   backup.SnapshotPreview
}

func (r *previewReaderFixture) PreviewSnapshot(_ context.Context, snapshot string, policy backup.RestorePolicy) (backup.SnapshotPreview, error) {
	r.snapshot, r.policy = snapshot, policy
	return r.result, nil
}

func TestPreviewWorkerRechecksAuthorityAndUsesInstalledPolicy(t *testing.T) {
	request := backup.SnapshotPreviewRequest{Version: 5, Kind: "snapshot-preview", RequestID: state.Random(), DeviceID: state.Random(), SnapshotID: state.Hash("selected")}
	authority := &previewAuthorityFixture{request: request, err: errors.New("withdrawn")}
	reader := &previewReaderFixture{result: backup.SnapshotPreview{SnapshotID: request.SnapshotID, Files: []backup.SnapshotPreviewFile{{Workload: "management", Bytes: 1}}}}
	policy := backup.RestorePolicy{MinimumCatalogVersion: 42}
	result, err := authorizedSnapshotPreview(context.Background(), request, authority, reader, policy)
	if err == nil || result.Files != nil || authority.calls != 1 || reader.snapshot != request.SnapshotID || reader.policy.MinimumCatalogVersion != 42 {
		t.Fatal("unqualified preview returned", result, err)
	}
	file, err := os.CreateTemp(t.TempDir(), "credential")
	if err != nil {
		t.Fatal(err)
	}
	config := BackupWorkerConfig{Policy: policy, RepositoryTarget: backup.Target{MountPath: "/mnt/homenode-backup", UUID: "fixture-drive", RepositoryID: state.Hash("repository")}}
	result, err = RunCredentialedSnapshotPreview(context.Background(), request, config, file, authority)
	if err == nil || result.Files != nil || authority.calls != 2 {
		t.Fatal("authorization did not precede credential read", result, err)
	}
	if _, err := file.Stat(); err == nil {
		t.Fatal("worker retained owned credential")
	}
}
