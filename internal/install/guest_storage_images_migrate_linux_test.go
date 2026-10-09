//go:build linux

package install

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/catalog"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

func TestRootGuestStorageImagesMigrationRecoversInterruptedBatch(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("owned disposable root fixture")
	}
	host, journal := roots(t)
	e := openEngine(t, host, journal)
	defer e.Close()
	ctx := context.Background()
	identity, err := guestUIDProvisioningPlan(ctx, strings.Repeat("a", 32), supervisor.GuestUIDPool{First: 200000, Last: 200002}, []uint32{1001, 1002})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := guestStorageProvisioningPlan(ctx, identity, 994, []int{1001, 1002, 1003})
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(host, "var/lib/homenode/images")
	if err := os.MkdirAll(directory, 0710); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(directory, 0, 993); err != nil {
		t.Fatal(err)
	}
	var manifest catalog.Manifest
	for _, data := range []string{"files image", "ai image", "convert image"} {
		hash := sha256.Sum256([]byte(data))
		image := catalog.Image{SHA256: hex.EncodeToString(hash[:]), Bytes: int64(len(data))}
		path := filepath.Join(directory, image.SHA256+".raw")
		if err := os.WriteFile(path, []byte(data), 0440); err != nil {
			t.Fatal(err)
		}
		if err := os.Chown(path, 0, 993); err != nil {
			t.Fatal(err)
		}
		manifest.Images = append(manifest.Images, image)
	}
	failure := errors.New("interrupted after first image migration")
	e.checkpoint = func(phase, path string) error {
		if phase == "guest-storage-image-migrated" {
			return failure
		}
		return nil
	}
	check := func(ctx context.Context) error { return ctx.Err() }
	if err := e.migrateGuestStorageImagesLocked(ctx, plan, manifest, 993, check); !errors.Is(err, failure) {
		t.Fatal("interruption lost", err)
	}
	saved, err := os.ReadFile(filepath.Join(journal, "guest-storage-images-intent.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	e = openEngine(t, host, journal)
	defer e.Close()
	if err := e.migrateGuestStorageImagesLocked(ctx, plan, manifest, 993, check); err != nil {
		t.Fatal("interrupted batch retry refused", err)
	}
	if err := e.migrateGuestStorageImagesLocked(ctx, plan, manifest, 993, check); err != nil {
		t.Fatal("completed batch retry refused", err)
	}
	migrated := 0
	e.checkpoint = func(phase, path string) error {
		if phase == "guest-storage-image-migrated" {
			migrated++
			if migrated == len(manifest.Images) {
				return os.Chown(filepath.Join(directory, manifest.Images[0].SHA256+".raw"), 0, 993)
			}
		}
		return nil
	}
	if err := e.migrateGuestStorageImagesLocked(ctx, plan, manifest, 993, check); !errors.Is(err, ErrConflict) {
		t.Fatal("batch completion admitted reverted image ownership", err)
	}
	e.checkpoint = nil
	if err := e.migrateGuestStorageImagesLocked(ctx, plan, manifest, 993, check); err != nil {
		t.Fatal("recorded reverted ownership could not reconcile", err)
	}
	current, err := os.ReadFile(filepath.Join(journal, "guest-storage-images-intent.json"))
	if err != nil || !bytes.Equal(saved, current) {
		t.Fatal("retry changed provenance", err)
	}
	for _, image := range manifest.Images {
		file, err := os.Open(filepath.Join(directory, image.SHA256+".raw"))
		if err != nil {
			t.Fatal(err)
		}
		_, err = qualifyGuestStorageImage(ctx, plan, image, 994, file)
		closeErr := file.Close()
		if err != nil || closeErr != nil {
			t.Fatal("migrated image not qualified", err, closeErr)
		}
	}
	path := filepath.Join(directory, manifest.Images[0].SHA256+".raw")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, path+".original"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0440); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(path, 0, 994); err != nil {
		t.Fatal(err)
	}
	mutations := 0
	e.checkpoint = func(phase, path string) error {
		if phase == "guest-storage-image-migrated" {
			mutations++
		}
		return nil
	}
	if err := e.migrateGuestStorageImagesLocked(ctx, plan, manifest, 993, check); !errors.Is(err, ErrConflict) || mutations != 0 {
		t.Fatal("replacement inode adopted", mutations, err)
	}
	for _, retained := range []string{path, path + ".original"} {
		current, err := os.ReadFile(retained)
		if err != nil || !bytes.Equal(current, data) {
			t.Fatal("refusal modified retained image", err)
		}
	}
	if err := os.Rename(path, path+".foreign"); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path+".original", path); err != nil {
		t.Fatal(err)
	}
	intentPath := filepath.Join(journal, "guest-storage-images-intent.json")
	if err := os.Rename(intentPath, intentPath+".original"); err != nil {
		t.Fatal(err)
	}
	if err := e.migrateGuestStorageImagesLocked(ctx, plan, manifest, 993, check); !errors.Is(err, ErrConflict) || mutations != 0 {
		t.Fatal("unrecorded destination ownership adopted", mutations, err)
	}
	if _, err := os.Lstat(intentPath); !os.IsNotExist(err) {
		t.Fatal("missing provenance synthesized", err)
	}
	retained, err := os.ReadFile(intentPath + ".original")
	if err != nil || !bytes.Equal(retained, saved) {
		t.Fatal("retained provenance changed", err)
	}
}
