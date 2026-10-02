package state

import (
	"context"
	"path/filepath"
	"testing"
)

func TestBackupReminderPreferencePersistsAcrossDatabaseRestart(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "state")
	store, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { store.Close() }()
	ctx := context.Background()
	if days, err := store.BackupReminderInterval(ctx); err != nil || days != 7 {
		t.Fatal(days, err)
	}
	device := Random()
	if _, err = store.DB.Exec("INSERT INTO devices(id,name,capabilities,created_at) VALUES(?,'owner','[\"admin\"]',1)", device); err != nil {
		t.Fatal(err)
	}
	if err = store.SetBackupReminderInterval(ctx, device, 30); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	if days, err := store.BackupReminderInterval(ctx); err != nil || days != 30 {
		t.Fatal("preference lost on restart", days, err)
	}
	if err = store.SetBackupReminderInterval(ctx, Random(), 14); err == nil {
		t.Fatal("foreign device changed preference")
	}
	if days, err := store.BackupReminderInterval(ctx); err != nil || days != 30 {
		t.Fatal("refusal mutated preference", days, err)
	}
}
