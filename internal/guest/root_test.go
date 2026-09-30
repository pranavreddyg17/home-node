package guest

import (
	"os"
	"path/filepath"
	"runtime"
	"syscall"
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

func TestNativeForeignDataRootPreserved(t *testing.T) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 || os.Getenv("HOMENODE_GUEST_ROOT_INTEGRATION") != "1" {
		t.Skip("requires opt-in disposable Linux root ownership fixture")
	}
	directory := privateDataDir(t)
	marker := filepath.Join(directory, "preserved")
	if err := os.WriteFile(marker, []byte("existing guest data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(directory, 900, 900); err != nil {
		t.Fatal(err)
	}
	if a, err := New(directory, "files", 1<<30); err == nil {
		a.Close()
		t.Fatal("foreign-owned root accepted")
	}
	info, err := os.Stat(directory)
	if err != nil {
		t.Fatal(err)
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !ok || owner.Uid != 900 || owner.Gid != 900 || info.Mode().Perm() != 0700 {
		t.Fatal("rejected root ownership or mode changed")
	}
	data, err := os.ReadFile(marker)
	if err != nil || string(data) != "existing guest data" {
		t.Fatal("rejected root data changed")
	}
}
