package backup

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestManagementOutputRequiresReboundJournalIdentity(t *testing.T) {
	source, manifest, _, _ := recoverySet(t)
	entry := manifest.Files[1]
	disk := RecoveryInstallDisk{Workload: entry.Workload, SourceName: entry.Name, Bytes: entry.Bytes, SourceSHA256: entry.SHA256, ImageSHA256: entry.ImageSHA256, InstanceID: state.Random()}
	plan, err := newRecoveryInstallPlan(context.Background(), source, strings.Repeat("c", 64), manifest, []RecoveryInstallDisk{disk})
	if err != nil {
		t.Fatal(err)
	}
	path := t.TempDir()
	if err = os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err = createRecoveryInstallPlan(context.Background(), root, plan); err != nil {
		t.Fatal(err)
	}
	data, err := source.ReadFile("snapshot.db")
	if err != nil {
		t.Fatal(err)
	}
	if err = root.WriteFile(".recovery-management.stage", data, 0600); err != nil {
		t.Fatal(err)
	}
	if output, err := inspectReboundRecoveryManagement(context.Background(), root); err == nil || output.SHA256 != "" {
		t.Fatal("historical database became rebound output", output, err)
	}
	db, err := sql.Open("sqlite", filepath.Join(path, ".recovery-management.stage"))
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err = state.RebindRecoveryTx(context.Background(), tx, plan.OwnerID, plan.RecoveryEpoch, map[string]string{"files": disk.InstanceID}); err != nil {
		tx.Rollback()
		db.Close()
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := inspectReboundRecoveryManagement(context.Background(), root)
	if err != nil || output.Bytes <= 0 || !repositoryPattern.MatchString(output.SHA256) || !repositoryPattern.MatchString(output.PlanSHA256) || output.SHA256 == manifest.Files[0].SHA256 {
		t.Fatal("rebound output identity differs", output, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if output, err := inspectReboundRecoveryManagement(ctx, root); err == nil || output.SHA256 != "" {
		t.Fatal("cancelled inspection returned identity", output, err)
	}
}
