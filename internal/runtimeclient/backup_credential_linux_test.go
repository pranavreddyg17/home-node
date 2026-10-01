//go:build linux

package runtimeclient

import (
	"context"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/backup"
)

func TestCredentialedWorkerClosesOwnedDescriptorOnRefusal(t *testing.T) {
	credential, err := backup.CreateRepositoryPassword([]byte("fixture-only-credential"))
	if err != nil {
		t.Fatal(err)
	}
	defer credential.Close()
	result, err := RunCredentialedDispatchedBackup(context.Background(), backup.Dispatch{}, BackupWorkerConfig{}, credential)
	if err == nil || result != (backup.BackupResult{}) {
		t.Fatal("invalid credentialed job admitted")
	}
	if _, err = credential.Stat(); err == nil {
		t.Fatal("refusal retained credential descriptor")
	}
}
