//go:build linux

package updates

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestInspectionLaunchPublicationBindsParentAndRetainsState(t *testing.T) {
	parentPath := t.TempDir()
	if err := os.Chmod(parentPath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(parentPath, "inspection"), 0700); err != nil {
		t.Fatal(err)
	}
	parent, err := os.OpenRoot(parentPath)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	root, err := parent.OpenRoot("inspection")
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	data := []byte("verified launch fixture")
	sourcePath := filepath.Join(t.TempDir(), "source.deb")
	if err := os.WriteFile(sourcePath, data, 0400); err != nil {
		t.Fatal(err)
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	sum := sha256.Sum256(data)
	identity := InspectionIdentity{OperationID: "inspection-fixture-000001", Release: "0.1.0", PackageSHA256: hex.EncodeToString(sum[:]), PackageLength: int64(len(data))}
	release := &AcquiredRelease{Package: source, PackageSHA256: identity.PackageSHA256, PackageLength: identity.PackageLength, Metadata: ReleaseMetadata{Release: identity.Release}}
	ctx := context.Background()
	if err := stageInspectionPackageOwned(ctx, root, release, identity.OperationID); err != nil {
		t.Fatal(err)
	}
	stage, err := openInspectionStageOwned(ctx, root, identity)
	if err != nil {
		t.Fatal(err)
	}
	defer stage.Close()
	unrelatedPath := t.TempDir()
	if err := os.Chmod(unrelatedPath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(unrelatedPath, "inspection"), 0700); err != nil {
		t.Fatal(err)
	}
	unrelated, err := os.OpenRoot(unrelatedPath)
	if err != nil {
		t.Fatal(err)
	}
	defer unrelated.Close()
	if err := stage.publishEnvironmentOwned(ctx, unrelated); err == nil {
		t.Fatal("unrelated package path configured")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := stage.publishEnvironmentOwned(canceled, parent); err == nil {
		t.Fatal("canceled publication accepted")
	}
	if _, err := os.Lstat(filepath.Join(parentPath, "inspection.env")); !os.IsNotExist(err) {
		t.Fatal("rejected publication changed state", err)
	}
	if err := stage.publishEnvironmentOwned(ctx, parent); err != nil {
		t.Fatal(err)
	}
	expected, err := stage.Environment()
	if err != nil {
		t.Fatal(err)
	}
	published, err := os.ReadFile(filepath.Join(parentPath, "inspection.env"))
	if err != nil || string(published) != string(expected) {
		t.Fatal("launch identity differs", err)
	}
	info, err := os.Lstat(filepath.Join(parentPath, "inspection.env"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("launch file permissions", err)
	}
	if err := stage.publishEnvironmentOwned(ctx, parent); err == nil {
		t.Fatal("existing launch configuration replaced")
	}
}
