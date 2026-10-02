package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func recoverySet(t *testing.T) (*os.Root, Manifest, RestorePolicy, string) {
	t.Helper()
	source, err := state.Open(filepath.Join(t.TempDir(), "source"))
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	if _, err = source.DB.Exec("INSERT INTO identity(singleton,owner_id,claimed) VALUES(1,'owner',1)"); err != nil {
		t.Fatal(err)
	}
	if _, err = source.DB.Exec("INSERT INTO apps(workload,instance_id,state,updated_at) VALUES('files',?,'running',1)", state.Random()); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(t.TempDir(), "recovery")
	if err = os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = source.RecoverySnapshot(context.Background(), directory); err != nil {
		t.Fatal(err)
	}
	// This tests metadata/byte reconciliation, not a valid guest filesystem image.
	if err = os.WriteFile(filepath.Join(directory, "files.raw"), []byte("guest disk integrity fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	m := Manifest{Version: 1, CreatedAt: time.Now(), Release: "0.1.0~dev", Platform: "ubuntu-24.04-amd64", ManagementSchema: 4, CatalogVersion: 1}
	image := strings.Repeat("a", 64)
	for _, entry := range []BackupFile{{Workload: "management", Name: "snapshot.db", DataSchema: 4}, {Workload: "files", Name: "files.raw", DataSchema: 1, Protocol: 1, ImageSHA256: image}} {
		data, err := os.ReadFile(filepath.Join(directory, entry.Name))
		if err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256(data)
		entry.Bytes = int64(len(data))
		entry.SHA256 = hex.EncodeToString(hash[:])
		m.Files = append(m.Files, entry)
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return root, m, RestorePolicy{MinimumCatalogVersion: 1, ApprovedImages: map[string]string{"files": image}}, directory
}
func TestRecoveryInventoryRequiresEveryDeclaredAppDisk(t *testing.T) {
	root, m, policy, _ := recoverySet(t)
	if err := ValidateRecoverySet(context.Background(), root, m, policy); err != nil {
		t.Fatal(err)
	}
	m.Files = m.Files[:1]
	if err := ValidateRecoverySet(context.Background(), root, m, policy); err == nil {
		t.Fatal("missing app disk accepted")
	}
}
func TestRecoveryRejectsRestoredAuthorityAndUnexpectedSchema(t *testing.T) {
	for _, mutation := range []string{"UPDATE identity SET claimed=1", "UPDATE apps SET state='running'", "CREATE VIEW injected AS SELECT 1"} {
		t.Run(mutation, func(t *testing.T) {
			root, m, policy, directory := recoverySet(t)
			db, err := sql.Open("sqlite", filepath.Join(directory, "snapshot.db"))
			if err != nil {
				t.Fatal(err)
			}
			_, err = db.Exec(mutation)
			_ = db.Close()
			if err != nil {
				t.Fatal(err)
			}
			// Recompute declared integrity: authority/schema checks must reject even an
			// honestly checksummed incompatible snapshot, not only random corruption.
			data, err := os.ReadFile(filepath.Join(directory, "snapshot.db"))
			if err != nil {
				t.Fatal(err)
			}
			hash := sha256.Sum256(data)
			m.Files[0].SHA256 = hex.EncodeToString(hash[:])
			m.Files[0].Bytes = int64(len(data))
			if err = ValidateRecoverySet(context.Background(), root, m, policy); err == nil {
				t.Fatal("restored authority/schema accepted")
			}
		})
	}
}

func TestRecoveryDiskQualificationRejectsChecksummedNonFilesystem(t *testing.T) {
	root, manifest, policy, directory := recoverySet(t)
	before, err := os.ReadFile(filepath.Join(directory, "files.raw"))
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateRecoverySet(context.Background(), root, manifest, policy); err != nil {
		t.Fatal("metadata fixture refused", err)
	}
	if err := QualifyRecoveryDisks(context.Background(), root, manifest, policy); err == nil {
		t.Fatal("checksummed non-filesystem qualified")
	}
	after, err := os.ReadFile(filepath.Join(directory, "files.raw"))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("qualification changed restored disk", err)
	}
}
