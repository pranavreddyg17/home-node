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
			if err = stageInspectionPackageOwned(ctx, root, release, "inspection-fixture-000002"); err == nil {
				t.Fatal("existing operation replaced")
			}
		})
	}
}
