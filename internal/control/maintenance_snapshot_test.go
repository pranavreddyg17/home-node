package control

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestPrivateSnapshotIsSanitizedAndTemporaryStorageRemoved(t *testing.T) {
	server := testServer(t)
	ctx := context.Background()
	if _, err := server.Store.DB.Exec("UPDATE identity SET owner_id=?,claimed=1,epoch=1 WHERE singleton=1", state.Random()); err != nil {
		t.Fatal(err)
	}
	device := state.Random()
	if _, err := server.Store.DB.Exec("INSERT INTO devices(id,name,capabilities,created_at) VALUES(?,'owner','[\"admin\"]',1)", device); err != nil {
		t.Fatal(err)
	}
	token, _, err := server.Store.BeginMaintenanceJob(ctx, device)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	if err = server.writeMaintenanceSnapshot(ctx, token, response); err != nil {
		t.Fatal(err)
	}
	if response.Header().Get("Content-Type") != "application/vnd.homenode.recovery-snapshot" || response.Body.Len() == 0 {
		t.Fatal("snapshot not streamed")
	}
	path := filepath.Join(t.TempDir(), "snapshot.db")
	if err = os.WriteFile(path, response.Body.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err = state.ValidateRecoverySnapshot(ctx, file); err != nil {
		t.Fatal("snapshot retained authority", err)
	}
	var database string
	if err = server.Store.DB.QueryRow("SELECT file FROM pragma_database_list WHERE name='main'").Scan(&database); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Dir(database))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "backup-snapshot-") {
			t.Fatal("temporary plaintext snapshot retained")
		}
	}
	if err = server.Store.Transaction(ctx, state.RequireAdmission); err == nil {
		t.Fatal("snapshot released maintenance")
	}
}
