//go:build linux

package supervisor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func testReservedSystemImage(t *testing.T) {
	parent := volumeFixtureDir(t)
	d := Domain{ID: state.Random(), GuestUID: 1000000000, GuestGID: 64054}
	data := []byte("owned immutable image fixture")
	digest := sha256.Sum256(data)
	d.Image.SHA256, d.Image.Bytes = hex.EncodeToString(digest[:]), int64(len(data))
	if err := os.Chown(parent, 0, int(d.GuestGID)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parent, 0710); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(parent, d.Image.SHA256+".raw")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(path, 0, int(d.GuestGID)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0440); err != nil {
		t.Fatal(err)
	}
	file, err := openReservedSystemImage(context.Background(), parent, d)
	if err != nil {
		t.Fatal("qualified image admission", err)
	}
	if _, err := file.WriteAt([]byte("x"), 0); err == nil {
		t.Fatal("image admission returned writable descriptor")
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	stopped := func(ctx context.Context) error { return ctx.Err() }
	if err := withReservedSystemImage(context.Background(), parent, d, stopped, func(ctx context.Context, file *os.File, guard func(context.Context) error) error { return guard(ctx) }); err != nil {
		t.Fatal("retained image scope", err)
	}
	if err := withReservedSystemImage(context.Background(), parent, d, stopped, func(ctx context.Context, file *os.File, guard func(context.Context) error) error {
		moved := path + ".displaced"
		if err := os.Rename(path, moved); err != nil {
			return err
		}
		if err := os.Symlink(moved, path); err != nil {
			return errors.Join(err, os.Rename(moved, path))
		}
		guardErr := guard(ctx)
		if err := errors.Join(os.Remove(path), os.Rename(moved, path)); err != nil {
			t.Fatal("restore redirected image", err)
		}
		if !errors.Is(guardErr, ErrPolicy) {
			t.Fatal("image guard followed redirected inode", guardErr)
		}
		return guardErr
	}); !errors.Is(err, ErrPolicy) {
		t.Fatal("image scope lost redirection refusal", err)
	}
	if err := withReservedSystemImage(context.Background(), parent, d, stopped, func(ctx context.Context, file *os.File, guard func(context.Context) error) error {
		if err := os.Chmod(path, 0600); err != nil {
			return err
		}
		altered := append([]byte(nil), data...)
		altered[0] ^= 1
		if err := os.WriteFile(path, altered, 0600); err != nil {
			return err
		}
		return os.Chmod(path, 0440)
	}); !errors.Is(err, ErrPolicy) {
		t.Fatal("scope accepted content change with restored metadata", err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0440); err != nil {
		t.Fatal(err)
	}
	alias := path + ".alias"
	if err := os.Link(path, alias); err != nil {
		t.Fatal(err)
	}
	if got, err := openReservedSystemImage(context.Background(), parent, d); !errors.Is(err, ErrPolicy) || got != nil {
		if got != nil {
			got.Close()
		}
		t.Fatal("linked image admitted", err)
	}
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	changed := append([]byte(nil), data...)
	changed[0] ^= 1
	if err := os.WriteFile(path, changed, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0440); err != nil {
		t.Fatal(err)
	}
	if got, err := openReservedSystemImage(context.Background(), parent, d); !errors.Is(err, ErrPolicy) || got != nil {
		if got != nil {
			got.Close()
		}
		t.Fatal("corrupted catalog image admitted", err)
	}
	if actual, err := os.ReadFile(path); err != nil || string(actual) != string(changed) {
		t.Fatal("refusal changed image bytes", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if got, err := openReservedSystemImage(canceled, parent, d); !errors.Is(err, context.Canceled) || got != nil {
		if got != nil {
			got.Close()
		}
		t.Fatal("canceled image admission", err)
	}
}
