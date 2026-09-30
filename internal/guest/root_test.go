package guest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDataRootAdmissionPreservesUnsafeStorage(t *testing.T) {
	for _, mode := range []os.FileMode{0755, 0770, 0777} {
		t.Run(mode.String(), func(t *testing.T) {
			dir := privateDataDir(t)
			if err := os.Chmod(dir, mode); err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(dir, "preserved")
			if err := os.WriteFile(marker, []byte("existing data"), 0600); err != nil {
				t.Fatal(err)
			}
			if a, err := New(dir, "files", 1<<30); err == nil {
				a.Close()
				t.Fatal("nonprivate root accepted")
			}
			data, err := os.ReadFile(marker)
			if err != nil || string(data) != "existing data" {
				t.Fatal("storage changed on rejection")
			}
			info, err := os.Stat(dir)
			if err != nil || info.Mode().Perm() != mode {
				t.Fatal("unsafe permissions silently repaired")
			}
		})
	}
	parent := privateDataDir(t)
	target := privateDataDir(t)
	link := filepath.Join(parent, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if a, err := New(link, "files", 1<<30); err == nil {
		a.Close()
		t.Fatal("symlink root accepted")
	}
	missing := filepath.Join(parent, "missing", "objects")
	if a, err := New(missing, "files", 1<<30); err == nil {
		a.Close()
		t.Fatal("missing parents created")
	}
	if _, err := os.Stat(filepath.Dir(missing)); !os.IsNotExist(err) {
		t.Fatal("missing parent mutated")
	}
}

func privateDataDir(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	return directory
}
