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

func TestInspectionStagingPublishesOwnedIdentityWithoutReplacingState(t *testing.T) {
	for _, scenario := range []string{"valid", "digest-mismatch", "existing-intent", "public-directory", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			directory := t.TempDir()
			if err := os.Chmod(directory, 0700); err != nil {
				t.Fatal(err)
			}
			root, err := os.OpenRoot(directory)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			data := []byte("verified acquisition fixture")
			filename := filepath.Join(t.TempDir(), "verified.deb")
			if err = os.WriteFile(filename, data, 0400); err != nil {
				t.Fatal(err)
			}
			file, err := os.Open(filename)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			sum := sha256.Sum256(data)
			release := &AcquiredRelease{Package: file, PackageSHA256: hex.EncodeToString(sum[:]), PackageLength: int64(len(data)), Metadata: ReleaseMetadata{Release: "0.1.0"}}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch scenario {
			case "digest-mismatch":
				release.PackageSHA256 = hex.EncodeToString(make([]byte, 32))
			case "existing-intent":
				if err = os.WriteFile(filepath.Join(directory, "intent"), []byte("retained"), 0600); err != nil {
					t.Fatal(err)
				}
			case "public-directory":
				if err = os.Chmod(directory, 0755); err != nil {
					t.Fatal(err)
				}
			case "canceled":
				cancel()
			}
			err = stageInspectionPackageOwned(ctx, root, release, "inspection-fixture-000001")
			if scenario != "valid" {
				if err == nil {
					t.Fatal("unsafe staging accepted")
				}
				if _, err = os.Lstat(filepath.Join(directory, "package.deb")); !os.IsNotExist(err) {
					t.Fatal("failed operation published package", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			published, err := os.ReadFile(filepath.Join(directory, "package.deb"))
			if err != nil || string(published) != string(data) {
				t.Fatal(err)
			}
			intent, err := os.ReadFile(filepath.Join(directory, "intent"))
			if err != nil {
				t.Fatal(err)
			}
			ready, err := os.ReadFile(filepath.Join(directory, "ready"))
			if err != nil || string(ready) != string(intent) {
				t.Fatal("ready identity mismatch", err)
			}
			identity := InspectionIdentity{OperationID: "inspection-fixture-000001", Release: release.Metadata.Release, PackageSHA256: release.PackageSHA256, PackageLength: release.PackageLength}
			stage, err := openInspectionStageOwned(ctx, root, identity)
			if err != nil {
				t.Fatal("ready stage refused", err)
			}
			duplicate, err := stage.DuplicatePackage()
			if err != nil {
				t.Fatal(err)
			}
			if digest, length, err := PackageIdentity(ctx, duplicate); err != nil || digest != identity.PackageSHA256 || length != identity.PackageLength {
				t.Fatal("worker descriptor differs", err)
			}
			if err := duplicate.Close(); err != nil {
				t.Fatal(err)
			}
			output, err := json.Marshal(InspectionResult{Schema: 1, OperationID: identity.OperationID, Release: identity.Release, PackageSHA256: identity.PackageSHA256, PackageLength: identity.PackageLength, ContentValid: true})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := stage.VerifyResult(ctx, output); err != nil {
				t.Fatal("matching output refused", err)
			}
			if _, err := stage.VerifyResult(ctx, []byte(`{"schema":1}`)); err == nil {
				t.Fatal("incomplete output accepted")
			}
			if other, err := openInspectionStageOwned(ctx, root, identity); err == nil {
				other.Close()
				t.Fatal("concurrent worker admitted")
			}
			packagePath := filepath.Join(directory, "package.deb")
			if err := os.Chmod(packagePath, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(packagePath, []byte("changed after worker completion"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := stage.VerifyResult(ctx, output); err == nil {
				t.Fatal("changed pinned package accepted with old result")
			}
			if err := os.WriteFile(packagePath, data, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(packagePath, 0400); err != nil {
				t.Fatal(err)
			}
			if err := stage.Close(); err != nil {
				t.Fatal(err)
			}
			if err := stage.Close(); err != nil {
				t.Fatal("repeated close failed", err)
			}
			if _, err := stage.Environment(); err == nil {
				t.Fatal("closed stage produced launch inputs")
			}
			if _, err := stage.VerifyResult(ctx, output); err == nil {
				t.Fatal("closed stage verified output")
			}
			if descriptor, err := stage.DuplicatePackage(); err == nil {
				descriptor.Close()
				t.Fatal("closed stage produced worker descriptor")
			}
			wrong := identity
			wrong.OperationID = "inspection-fixture-000002"
			if other, err := openInspectionStageOwned(ctx, root, wrong); err == nil {
				other.Close()
				t.Fatal("different operation admitted")
			}
			if err := os.WriteFile(filepath.Join(directory, "ready"), []byte("tampered"), 0600); err != nil {
				t.Fatal(err)
			}
			if other, err := openInspectionStageOwned(ctx, root, identity); err == nil {
				other.Close()
				t.Fatal("tampered ready admitted")
			}
			if err = stageInspectionPackageOwned(ctx, root, release, "inspection-fixture-000002"); err == nil {
				t.Fatal("existing operation replaced")
			}
		})
	}
}
