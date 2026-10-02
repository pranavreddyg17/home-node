//go:build linux

package updates

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	pendingPath := filepath.Join(parentPath, "inspection.env.pending")
	if err := os.WriteFile(pendingPath, []byte("interrupted launch"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := stage.publishEnvironmentOwned(ctx, parent); err == nil {
		t.Fatal("interrupted launch replaced")
	}
	retained, err := os.ReadFile(pendingPath)
	if err != nil || string(retained) != "interrupted launch" {
		t.Fatal("pending evidence changed", err)
	}
	if _, err := os.Lstat(filepath.Join(parentPath, "inspection.env")); !os.IsNotExist(err) {
		t.Fatal("interrupted launch published", err)
	}
	// Only this disposable fixture explicitly removes interrupted state before
	// exercising a new successful publication.
	if err := os.Remove(pendingPath); err != nil {
		t.Fatal(err)
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
	if err := stage.verifyEnvironmentOwned(ctx, parent); err != nil {
		t.Fatal("published configuration refused", err)
	}
	epoch, err := CaptureInspectionLaunchEpoch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := stage.publishLaunchIntentOwned(canceled, parent, epoch); err == nil {
		t.Fatal("canceled launch intent published")
	}
	if err := stage.publishLaunchIntentOwned(ctx, unrelated, epoch); err == nil {
		t.Fatal("unrelated parent received launch intent")
	}
	wrong := epoch
	wrong.NotBeforeMicros = ^uint64(0)
	if err := stage.publishLaunchIntentOwned(ctx, parent, wrong); err == nil {
		t.Fatal("future launch intent published")
	}
	launchPath := filepath.Join(parentPath, "inspection.launch")
	if _, err := os.Lstat(launchPath); !os.IsNotExist(err) {
		t.Fatal("rejected launch admission changed state", err)
	}
	launchPending := filepath.Join(parentPath, "inspection.launch.pending")
	if err := os.WriteFile(launchPending, []byte("interrupted"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := stage.publishLaunchIntentOwned(ctx, parent, epoch); err == nil {
		t.Fatal("interrupted launch intent overwritten")
	}
	if retained, err := os.ReadFile(launchPending); err != nil || string(retained) != "interrupted" {
		t.Fatal("interrupted evidence changed", err)
	}
	if err := os.Remove(launchPending); err != nil {
		t.Fatal(err)
	}
	if err := stage.publishLaunchIntentOwned(ctx, parent, epoch); err != nil {
		t.Fatal(err)
	}
	launch, err := os.ReadFile(launchPath)
	if err != nil || len(launch) == 0 {
		t.Fatal("launch intent missing", err)
	}
	var recorded map[string]json.RawMessage
	wanted := map[string]any{"schema": 1, "operationId": identity.OperationID, "release": identity.Release, "packageSha256": identity.PackageSHA256, "packageLength": identity.PackageLength, "bootId": epoch.BootID, "notBeforeMicros": epoch.NotBeforeMicros}
	if err := json.Unmarshal(launch, &recorded); err != nil || len(recorded) != len(wanted) {
		t.Fatal("launch record shape", err)
	}
	for key, value := range wanted {
		encoded, err := json.Marshal(value)
		if err != nil || string(recorded[key]) != string(encoded) {
			t.Fatal("launch record identity mismatch", key, err)
		}
	}
	if info, err := os.Lstat(launchPath); err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("launch intent permissions", err)
	}
	if err := stage.publishLaunchIntentOwned(ctx, parent, epoch); err == nil {
		t.Fatal("existing launch intent overwritten")
	}
	if retained, err := os.ReadFile(launchPath); err != nil || string(retained) != string(launch) {
		t.Fatal("launch evidence changed on retry", err)
	}
	packagePath := filepath.Join(parentPath, "inspection/package.deb")
	retainedPath := filepath.Join(parentPath, "inspection/package.retained")
	if err := os.Rename(packagePath, retainedPath); err != nil {
		t.Fatal(err)
	}
	// Even identical bytes in a replacement inode must not be substituted for
	// the descriptor admitted under the operation's execution lock.
	if err := os.WriteFile(packagePath, data, 0400); err != nil {
		t.Fatal(err)
	}
	if err := stage.verifyEnvironmentOwned(ctx, parent); err == nil {
		t.Fatal("replacement service package inode accepted")
	}
	if err := os.Remove(packagePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(retainedPath, packagePath); err != nil {
		t.Fatal(err)
	}
	if err := stage.verifyEnvironmentOwned(ctx, parent); err != nil {
		t.Fatal("restored pinned service path refused", err)
	}
	if err := stage.verifyEnvironmentOwned(ctx, unrelated); err == nil {
		t.Fatal("unrelated launch configuration accepted")
	}
	if err := os.WriteFile(filepath.Join(parentPath, "inspection.env"), []byte("INSPECTION_RELEASE=0.2.0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := stage.verifyEnvironmentOwned(ctx, parent); err == nil {
		t.Fatal("changed configuration accepted")
	}
	if err := os.WriteFile(filepath.Join(parentPath, "inspection.env"), expected, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pendingPath, []byte("interrupted"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := stage.verifyEnvironmentOwned(ctx, parent); err == nil {
		t.Fatal("conflicting pending launch accepted")
	}
	if err := os.Remove(pendingPath); err != nil {
		t.Fatal(err)
	}
	if err := stage.publishEnvironmentOwned(ctx, parent); err == nil {
		t.Fatal("existing launch configuration replaced")
	}
	if _, err := os.Lstat(pendingPath); !os.IsNotExist(err) {
		t.Fatal("occupied publication created pending state", err)
	}
}
