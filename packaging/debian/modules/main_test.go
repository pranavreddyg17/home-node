package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestActualExecutableBuildInfoAndHash(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "binary"), data, 0600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	record, err := inspect(root, "binary")
	if err != nil {
		t.Fatal(err)
	}
	expected := sha256.Sum256(data)
	if record.SHA256 != hex.EncodeToString(expected[:]) || record.GoVersion != runtime.Version() || record.Path != "binary" {
		t.Fatal("actual executable identity mismatch", record)
	}
	if err := os.WriteFile(filepath.Join(directory, "foreign"), []byte("not a Go executable"), 0600); err != nil {
		t.Fatal(err)
	}
	if record, err := inspect(root, "foreign"); err == nil || record.SHA256 != "" {
		t.Fatal("foreign bytes admitted", err)
	}
}
