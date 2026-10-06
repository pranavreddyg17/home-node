package install

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/backup"
)

func TestRecoveryCopyChecksBorrowedBytesAndPreservesOccupiedStage(t *testing.T) {
	path := t.TempDir()
	if err := os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	payload := []byte("restored disconnected management bytes")
	if err = root.WriteFile("source", payload, 0600); err != nil {
		t.Fatal(err)
	}
	source, err := root.Open("source")
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	borrowed := backup.PreparedRecoveryFile{File: source, Bytes: int64(len(payload)), SHA256: digest(payload)}
	if _, err = source.Seek(3, 0); err != nil {
		t.Fatal(err)
	}
	name := ".recovery-management.copy"
	if err = copyRecoveryFile(context.Background(), root, name, borrowed, os.Geteuid()); err != nil {
		t.Fatal(err)
	}
	if offset, err := source.Seek(0, 1); err != nil || offset != 3 {
		t.Fatal("borrowed offset changed", offset, err)
	}
	got, err := root.ReadFile(name)
	if err != nil || string(got) != string(payload) {
		t.Fatal("copy bytes", string(got), err)
	}
	if err = verifyRecoveryCopy(context.Background(), root, name, borrowed, os.Geteuid()); err != nil {
		t.Fatal("completed copy verification", err)
	}
	if err = root.Chmod(name, 0644); err != nil {
		t.Fatal(err)
	}
	if err = verifyRecoveryCopy(context.Background(), root, name, borrowed, os.Geteuid()); !errors.Is(err, ErrConflict) {
		t.Fatal("permissive copy accepted", err)
	}
	if err = root.Chmod(name, 0600); err != nil {
		t.Fatal(err)
	}
	if err = copyRecoveryFile(context.Background(), root, name, borrowed, os.Geteuid()); !errors.Is(err, os.ErrExist) {
		t.Fatal("occupied stage overwritten", err)
	}
	if err = root.Remove(name); err != nil {
		t.Fatal(err)
	}
	borrowed.SHA256 = digest([]byte("foreign bytes"))
	if err = copyRecoveryFile(context.Background(), root, name, borrowed, os.Geteuid()); !errors.Is(err, ErrConflict) {
		t.Fatal("checksum mismatch accepted", err)
	}
	if err = verifyRecoveryCopy(context.Background(), root, name, borrowed, os.Geteuid()); !errors.Is(err, ErrConflict) {
		t.Fatal("wrong copy digest admitted", err)
	}
	got, err = root.ReadFile(name)
	if err != nil || string(got) != string(payload) {
		t.Fatal("uncertain copy bytes lost", err)
	}
	if err = copyRecoveryFile(context.Background(), root, "../escape", borrowed, os.Geteuid()); !errors.Is(err, ErrPlan) {
		t.Fatal("foreign stage path accepted", err)
	}
	if err = root.Remove(name); err != nil {
		t.Fatal(err)
	}
	borrowed.SHA256 = digest(payload)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err = copyRecoveryFile(cancelled, root, name, borrowed, os.Geteuid()); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled copy proceeded", err)
	}
	if _, err = root.Lstat(name); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("cancelled copy created stage", err)
	}
	if err = root.Symlink("source", name); err != nil {
		t.Fatal(err)
	}
	if err = verifyRecoveryCopy(context.Background(), root, name, borrowed, os.Geteuid()); !errors.Is(err, ErrConflict) {
		t.Fatal("symlink stage admitted", err)
	}
	if err = copyRecoveryFile(context.Background(), root, name, borrowed, os.Geteuid()); !errors.Is(err, os.ErrExist) {
		t.Fatal("occupied symlink replaced", err)
	}
	got, err = root.ReadFile("source")
	if err != nil || string(got) != string(payload) {
		t.Fatal("symlink target altered", err)
	}
	if err = root.Remove(name); err != nil {
		t.Fatal(err)
	}
	if err = root.WriteFile(name, payload[:3], 0600); err != nil {
		t.Fatal(err)
	}
	if err = verifyRecoveryCopy(context.Background(), root, name, borrowed, os.Geteuid()); !errors.Is(err, ErrConflict) {
		t.Fatal("partial copy admitted", err)
	}
	got, err = root.ReadFile(name)
	if err != nil || string(got) != string(payload[:3]) {
		t.Fatal("partial copy changed during refusal", err)
	}
}
