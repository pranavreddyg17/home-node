//go:build linux

package install

import (
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

func TestRootGuestStorageImageIdentity(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("owned disposable root fixture")
	}
	ctx := context.Background()
	identity, err := guestUIDProvisioningPlan(ctx, strings.Repeat("a", 32), supervisor.GuestUIDPool{First: 200000, Last: 200002}, []uint32{1001, 1002})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := guestStorageProvisioningPlan(ctx, identity, 994, []int{1001, 1002, 1003})
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("authenticated storage image")
	hash := sha256.Sum256(data)
	image := catalog.Image{SHA256: hex.EncodeToString(hash[:]), Bytes: int64(len(data))}
	path := filepath.Join(t.TempDir(), image.SHA256+".raw")
	if err := os.WriteFile(path, data, 0440); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(path, 0, 993); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	got, err := qualifyGuestStorageImage(ctx, plan, image, 993, file)
	if err != nil || got.Inode == 0 || got.OwnerID != identity.OwnerID || got.GuestGID != 994 {
		t.Fatal("qualified image identity", got, err)
	}
	if err := os.Link(path, path+".alias"); err != nil {
		t.Fatal(err)
	}
	if got, err := qualifyGuestStorageImage(ctx, plan, image, 993, file); !errors.Is(err, ErrConflict) || got != (guestStorageImageIdentity{}) {
		t.Fatal("multiply linked image admitted", got, err)
	}
	if err := os.Remove(path + ".alias"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("x", len(data))), 0440); err != nil {
		t.Fatal(err)
	}
	if got, err := qualifyGuestStorageImage(ctx, plan, image, 993, file); !errors.Is(err, catalog.ErrUntrusted) || got != (guestStorageImageIdentity{}) {
		t.Fatal("corrupt image admitted", got, err)
	}
	if err := os.WriteFile(path, data, 0440); err != nil {
		t.Fatal(err)
	}
	receipt, err := qualifyGuestStorageImage(ctx, plan, image, 993, file)
	if err != nil {
		t.Fatal(err)
	}
	refused := errors.New("migration exclusion revoked")
	if err := migrateGuestStorageImage(ctx, plan, receipt, file, func(context.Context) error { return refused }); !errors.Is(err, refused) {
		t.Fatal("revoked migration admitted", err)
	}
	if _, err := qualifyGuestStorageImage(ctx, plan, image, 993, file); err != nil {
		t.Fatal("refusal changed ownership", err)
	}
	check := func(ctx context.Context) error { return ctx.Err() }
	for attempt := 0; attempt < 2; attempt++ {
		if err := migrateGuestStorageImage(ctx, plan, receipt, file, check); err != nil {
			t.Fatal("recorded migration/retry refused", attempt, err)
		}
	}
	if _, err := qualifyGuestStorageImage(ctx, plan, image, 994, file); err != nil {
		t.Fatal("destination ownership not qualified", err)
	}
	foreign := receipt
	foreign.Inode++
	if err := migrateGuestStorageImage(ctx, plan, foreign, file, check); !errors.Is(err, ErrConflict) {
		t.Fatal("foreign provenance admitted", err)
	}
	for _, revokeAt := range []int{3, 4} {
		probes := 0
		if err := migrateGuestStorageImage(ctx, plan, receipt, file, func(ctx context.Context) error {
			probes++
			if probes == revokeAt {
				return file.Chown(0, int(receipt.SourceGID))
			}
			return ctx.Err()
		}); !errors.Is(err, ErrConflict) {
			t.Fatal("post-migration source ownership admitted", revokeAt, probes, err)
		}
	}
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	parent, err := root.Open(".")
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	if err := withGuestStorageImagePath(ctx, root, parent, image.SHA256, check, func(pinned *os.File, guard func(context.Context) error) error {
		return migrateGuestStorageImage(ctx, plan, receipt, pinned, guard)
	}); err != nil {
		t.Fatal("path-qualified migration refused", err)
	}
	if err := withGuestStorageImagePath(ctx, root, parent, image.SHA256, check, func(pinned *os.File, guard func(context.Context) error) error {
		if err := os.Rename(path, path+".original"); err != nil {
			return err
		}
		if err := os.WriteFile(path, data, 0440); err != nil {
			return err
		}
		if err := guard(ctx); !errors.Is(err, ErrConflict) {
			t.Fatal("replacement image path admitted", err)
		}
		return nil
	}); !errors.Is(err, ErrConflict) {
		t.Fatal("final path guard admitted replacement", err)
	}
	for _, retained := range []string{path, path + ".original"} {
		current, err := os.ReadFile(retained)
		if err != nil || string(current) != string(data) {
			t.Fatal("path refusal modified image", retained, err)
		}
	}
}
