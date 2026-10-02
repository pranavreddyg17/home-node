package control

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/backup"
	"github.com/pranavreddyg17/home-node/internal/state"
)

type refusedBackupRoot struct{ t *testing.T }

func (r refusedBackupRoot) BeginRuntimeMaintenanceForJob(context.Context, string) (string, error) {
	r.t.Fatal("unexpected root effect")
	return "", backup.ErrManifest
}
func (r refusedBackupRoot) EndRuntimeMaintenance(context.Context, string) error {
	r.t.Fatal("unexpected root release")
	return backup.ErrManifest
}

func TestApprovedBackupPreflightRefusesBeforeAdmission(t *testing.T) {
	s := testServer(t)
	s.config.BackupRepositoryID = strings.Repeat("a", 64)
	s.config.Runtime = fileBackend{}
	session := seedSession(t, s, `["admin"]`)
	actor, err := s.Identity.Authenticate(context.Background(), session)
	if err != nil {
		t.Fatal(err)
	}
	credential, err := os.CreateTemp(t.TempDir(), "disk-credential")
	if err != nil {
		t.Fatal(err)
	}
	defer credential.Close()
	if _, err = credential.WriteString("fixture-password"); err != nil {
		t.Fatal(err)
	}
	deliver := func(context.Context, backup.Dispatch, *os.File) error {
		t.Fatal("unexpected credential delivery")
		return backup.ErrManifest
	}
	for _, release := range []string{"0.1.0", "invalid-release"} {
		job, snapshot, err := s.RunApprovedBackup(context.Background(), actor, "unissued-grant", []byte(`{"repositoryId":"`+s.config.BackupRepositoryID+`"}`), "backup-request-1234567890", release, 1, credential, refusedBackupRoot{t}, deliver)
		if err == nil || job != "" || snapshot != "" {
			t.Fatal("invalid preflight exposed result", job, snapshot, err)
		}
		if err := s.Store.Transaction(context.Background(), state.RequireAdmission); err != nil {
			t.Fatal("preflight closed admission", err)
		}
	}
	if _, err = credential.Stat(); err != nil {
		t.Fatal("caller credential closed", err)
	}
}
