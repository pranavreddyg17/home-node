//go:build linux

package install

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/pranavreddyg17/home-node/internal/catalog"
	"os"
	"path/filepath"
	"testing"
)

func TestRootGuestStorageConfigurationImageRequiresReservedOwnership(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("disposable Linux root fixture required")
	}
	host, journalDir := roots(t)
	directory := filepath.Join(host, "var/lib/homenode/images")
	if err := os.MkdirAll(directory, 0710); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(directory, 0, 994); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0710); err != nil {
		t.Fatal(err)
	}
	data := []byte("signed fixture image")
	sum := sha256.Sum256(data)
	image := catalog.Image{SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(data))}
	path := filepath.Join(directory, image.SHA256+".raw")
	if err := os.WriteFile(path, data, 0440); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(path, 0, 994); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0440); err != nil {
		t.Fatal(err)
	}
	e := openEngine(t, host, journalDir)
	defer e.Close()
	guard := func(ctx context.Context) error { return ctx.Err() }
	if err := e.verifyGuestStorageConfigurationImage(context.Background(), image, 994, guard); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(path, 1, 994); err != nil {
		t.Fatal(err)
	}
	if err := e.verifyGuestStorageConfigurationImage(context.Background(), image, 994, guard); !errors.Is(err, ErrConflict) {
		t.Fatal("digest substituted for image ownership", err)
	}
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != string(data) {
		t.Fatal("refusal changed image", err)
	}
}
