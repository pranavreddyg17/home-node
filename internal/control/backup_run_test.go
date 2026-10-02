package control

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/backup"
	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestApprovedBackupPreflightRefusesBeforeAdmission(t *testing.T) {
	s := testServer(t)
	s.config.BackupRepositoryID = strings.Repeat("a", 64)
	s.config.Runtime = fileBackend{}
	session := seedBackupSession(t, s)
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
	deliver := func(context.Context, backup.Launch, *os.File) error {
		t.Fatal("unexpected credential delivery")
		return backup.ErrManifest
	}
	for _, release := range []string{"0.1.0", "invalid-release"} {
		job, snapshot, err := s.RunApprovedBackup(context.Background(), actor, "unissued-grant", []byte(`{"repositoryId":"`+s.config.BackupRepositoryID+`"}`), "backup-request-1234567890", release, 1, credential, deliver, refusedBackupCleanup(t))
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

func TestApprovedBackupPasswordClearsOwnedInputOnRefusal(t *testing.T) {
	s := testServer(t)
	s.config.BackupRepositoryID = strings.Repeat("a", 64)
	s.config.Runtime = fileBackend{}
	session := seedBackupSession(t, s)
	actor, err := s.Identity.Authenticate(context.Background(), session)
	if err != nil {
		t.Fatal(err)
	}
	deliver := func(context.Context, backup.Launch, *os.File) error {
		t.Fatal("unexpected delivery")
		return backup.ErrManifest
	}
	for _, scenario := range []string{"invalid-release", "unissued-grant", "cancelled", "invalid-password"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			release := "0.1.0"
			password := []byte("fixture-owned-password")
			if scenario == "invalid-release" {
				release = "invalid"
			}
			if scenario == "invalid-password" {
				password = []byte("fixture\nsecret")
			}
			if scenario == "cancelled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			job, snapshot, err := s.RunApprovedBackupPassword(ctx, actor, "unissued-grant", []byte(`{"repositoryId":"`+s.config.BackupRepositoryID+`"}`), "backup-request-1234567890", release, 1, password, deliver, refusedBackupCleanup(t))
			if err == nil || job != "" || snapshot != "" {
				t.Fatal("refused password launch exposed result", job, snapshot, err)
			}
			for _, value := range password {
				if value != 0 {
					t.Fatal("owned credential bytes retained")
				}
			}
			if err := s.Store.Transaction(context.Background(), state.RequireAdmission); err != nil {
				t.Fatal("refused launch closed admission", err)
			}
		})
	}
}

func refusedBackupCleanup(t *testing.T) func(context.Context, backup.Cleanup, *os.File) error {
	return func(context.Context, backup.Cleanup, *os.File) error {
		t.Fatal("unexpected worker cleanup")
		return backup.ErrManifest
	}
}

func TestApprovedBackupRequiresBothWorkerOperationsBeforeAdmission(t *testing.T) {
	s := testServer(t)
	s.config.BackupRepositoryID = strings.Repeat("a", 64)
	s.config.Runtime = fileBackend{}
	actor, err := s.Identity.Authenticate(context.Background(), seedBackupSession(t, s))
	if err != nil {
		t.Fatal(err)
	}
	credential, err := os.CreateTemp(t.TempDir(), "credential")
	if err != nil {
		t.Fatal(err)
	}
	defer credential.Close()
	launch := func(context.Context, backup.Launch, *os.File) error {
		t.Fatal("unexpected worker launch")
		return backup.ErrManifest
	}
	for _, missing := range []string{"launch", "cleanup"} {
		l, c := launch, refusedBackupCleanup(t)
		if missing == "launch" {
			l = nil
		} else {
			c = nil
		}
		job, snapshot, err := s.RunApprovedBackup(context.Background(), actor, "unissued", []byte(`{}`), "request-1234567890", "0.1.0", 1, credential, l, c)
		if err == nil || job != "" || snapshot != "" {
			t.Fatal("incomplete worker operations accepted", missing, err)
		}
		if err = s.Store.Transaction(context.Background(), state.RequireAdmission); err != nil {
			t.Fatal("incomplete operations closed admission", err)
		}
	}
	if _, err = credential.Stat(); err != nil {
		t.Fatal("caller credential closed", err)
	}
}

func TestAsyncApprovedBackupClearsPasswordOnRefusal(t *testing.T) {
	s := testServer(t)
	actor, err := s.Identity.Authenticate(context.Background(), seedBackupSession(t, s))
	if err != nil {
		t.Fatal(err)
	}
	launch := func(context.Context, backup.Launch, *os.File) error {
		t.Fatal("unexpected asynchronous launch")
		return backup.ErrManifest
	}
	for _, scenario := range []string{"cancelled", "invalid-password", "unconfigured"} {
		ctx := context.Background()
		password := []byte("owned-secret")
		if scenario == "cancelled" {
			var cancel context.CancelFunc
			ctx, cancel = context.WithCancel(ctx)
			cancel()
		}
		if scenario == "invalid-password" {
			password = []byte("secret\ninvalid")
		}
		job, err := s.StartApprovedBackupPassword(ctx, actor, "unissued", []byte(`{}`), "request-1234567890", "0.1.0", 1, password, launch, refusedBackupCleanup(t))
		if err == nil || job != "" {
			t.Fatal("refusal returned admitted job", job, err)
		}
		for _, b := range password {
			if b != 0 {
				t.Fatal("owned password retained")
			}
		}
		if err = s.Store.Transaction(context.Background(), state.RequireAdmission); err != nil {
			t.Fatal("refusal closed admission", err)
		}
	}
}

func TestAdmittedBackupTaskClosesCredentialOnFailure(t *testing.T) {
	s := testServer(t)
	credential, err := os.CreateTemp(t.TempDir(), "invalid-credential")
	if err != nil {
		t.Fatal(err)
	}
	launch := func(context.Context, backup.Launch, *os.File) error {
		t.Fatal("unexpected launch")
		return backup.ErrManifest
	}
	run := s.admittedBackupTask(state.Random(), state.Random(), state.MaintenanceJob{ID: state.Random()}, "0.1.0", 1, credential, launch, refusedBackupCleanup(t))
	if err = run(context.Background()); err == nil {
		t.Fatal("invalid credential accepted")
	}
	if _, err = credential.Stat(); err == nil {
		t.Fatal("task retained owned descriptor")
	}
}

func seedBackupSession(t *testing.T, s *Server) string {
	t.Helper()
	session, device := state.Random(), state.Random()
	now := time.Now().Unix()
	if _, err := s.Store.DB.Exec("INSERT INTO devices(id,name,capabilities,created_at) VALUES(?,'owner','[\"admin\"]',?)", device, now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Store.DB.Exec("INSERT INTO sessions VALUES(?,?,1,?,?,?,?)", state.Hash(session), device, now, now, now, now+3600); err != nil {
		t.Fatal(err)
	}
	return session
}
