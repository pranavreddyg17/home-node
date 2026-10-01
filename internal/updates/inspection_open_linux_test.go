//go:build linux

package updates

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestInspectionStageRejectsUnsafePublication(t *testing.T) {
	for _, scenario := range []string{"missing-ready", "extra-entry", "symlink", "hardlink", "fifo", "public-record", "writable-package", "changed-package", "oversized-ready", "public-directory", "canceled"} {
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
			data := []byte("verified inspection fixture")
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
			if err := stageInspectionPackageOwned(context.Background(), root, release, identity.OperationID); err != nil {
				t.Fatal(err)
			}
			ready := filepath.Join(directory, "ready")
			packagePath := filepath.Join(directory, "package.deb")
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			switch scenario {
			case "missing-ready":
				err = os.Remove(ready)
			case "extra-entry":
				err = os.WriteFile(filepath.Join(directory, "unknown"), nil, 0600)
			case "symlink":
				if err = os.Remove(ready); err == nil {
					err = os.Symlink(filepath.Join(directory, "intent"), ready)
				}
			case "hardlink":
				if err = os.Remove(ready); err == nil {
					err = os.Link(filepath.Join(directory, "intent"), ready)
				}
			case "fifo":
				if err = os.Remove(ready); err == nil {
					err = unix.Mkfifo(ready, 0600)
				}
			case "public-record":
				err = os.Chmod(ready, 0644)
			case "writable-package":
				err = os.Chmod(packagePath, 0600)
			case "changed-package":
				if err = os.Chmod(packagePath, 0600); err == nil {
					err = os.WriteFile(packagePath, []byte("corrupted inspection bytes"), 0600)
				}
				if err == nil {
					err = os.Chmod(packagePath, 0400)
				}
			case "oversized-ready":
				err = os.WriteFile(ready, make([]byte, 4096), 0600)
			case "public-directory":
				err = os.Chmod(directory, 0755)
			case "canceled":
				cancel()
			}
			if err != nil {
				t.Fatal(err)
			}
			started := time.Now()
			stage, err := openInspectionStageOwned(ctx, root, identity)
			if err == nil {
				stage.Close()
				t.Fatal("unsafe stage admitted")
			}
			if time.Since(started) > 500*time.Millisecond {
				t.Fatal("unsafe file blocked admission")
			}
			// Every refusal must release its directory lock so explicit recovery can
			// take ownership; it must not delete or silently repair durable evidence.
			lock, err := root.Open(".")
			if err != nil {
				t.Fatal(err)
			}
			defer lock.Close()
			if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
				t.Fatal("refusal leaked execution lock", err)
			}
			if scenario != "missing-ready" {
				if _, err := os.Lstat(ready); err != nil {
					t.Fatal("refusal removed evidence", err)
				}
			}
		})
	}
}
