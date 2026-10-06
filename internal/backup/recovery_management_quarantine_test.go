package backup

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestRecoveryManagementQuarantinePreservesUncertainBytes(t *testing.T) {
	source, manifest, _, _ := recoverySet(t)
	plan, err := newRecoveryInstallPlan(context.Background(), source, strings.Repeat("c", 64), manifest, []RecoveryInstallDisk{})
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
	const stage = ".recovery-management.stage"
	if err = root.WriteFile(stage, []byte("uncertain partial bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = root.WriteFile(stage+"-journal", []byte("hot journal"), 0600); err != nil {
		t.Fatal(err)
	}
	if name, err := quarantineRecoveryManagementStage(context.Background(), root, manifest); !errors.Is(err, ErrManifest) || name != "" {
		t.Fatal("SQLite journal state quarantined", name, err)
	}
	if err = root.Remove(stage + "-journal"); err != nil {
		t.Fatal(err)
	}
	name, err := quarantineRecoveryManagementStage(context.Background(), root, manifest)
	if err != nil {
		t.Fatal(err)
	}
	data, err := root.ReadFile(name)
	if err != nil || string(data) != "uncertain partial bytes" {
		t.Fatal("quarantine lost data", err)
	}
	if _, err = root.Lstat(stage); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("quarantined stage retained active name", err)
	}
	original, err := source.ReadFile("snapshot.db")
	if err != nil {
		t.Fatal(err)
	}
	if err = root.WriteFile(stage, original, 0600); err != nil {
		t.Fatal(err)
	}
	if name, err := quarantineRecoveryManagementStage(context.Background(), root, manifest); !errors.Is(err, ErrManifest) || name != "" {
		t.Fatal("completed original copy quarantined", name, err)
	}
}
