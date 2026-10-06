package supervisor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestGuestUIDAccountFileReadRejectsUnsafeInputs(t *testing.T) {
	for _, name := range []string{"passwd", "subuid", "nsswitch.conf"} {
		t.Run(name, func(t *testing.T) { testGuestUIDAccountFileReadRejectsUnsafeInputs(t, name) })
	}
}

func testGuestUIDAccountFileReadRejectsUnsafeInputs(t *testing.T, name string) {
	directory := t.TempDir()
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	path := filepath.Join(directory, name)
	content := []byte("root:x:0:0:root:/root:/bin/sh\n")
	if err = os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	if data, err := readGuestUIDAccountFile(context.Background(), root, name, uint32(os.Geteuid())); err != nil || string(data) != string(content) {
		t.Fatal("protected account read", err)
	}
	if _, err = readGuestUIDAccountFile(context.Background(), root, name, uint32(os.Geteuid()+1)); !errors.Is(err, ErrPolicy) {
		t.Fatal("foreign owner admitted", err)
	}
	if err = os.Chmod(path, 0666); err != nil {
		t.Fatal(err)
	}
	if _, err = readGuestUIDAccountFile(context.Background(), root, name, uint32(os.Geteuid())); !errors.Is(err, ErrPolicy) {
		t.Fatal("writable account file admitted", err)
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(directory, "target")
	if err = os.WriteFile(target, content, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if _, err = readGuestUIDAccountFile(context.Background(), root, name, uint32(os.Geteuid())); !errors.Is(err, ErrPolicy) {
		t.Fatal("symlink admitted", err)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != string(content) {
		t.Fatal("symlink target changed", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = readGuestUIDAccountFile(ctx, root, name, uint32(os.Geteuid())); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err = readGuestUIDAccountFile(context.Background(), root, "foreign", uint32(os.Geteuid())); !errors.Is(err, ErrPolicy) {
		t.Fatal(err)
	}
}
