package install

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBootstrapReplayAndLock(t *testing.T) {
	host := t.TempDir()
	if err := os.MkdirAll(filepath.Join(host, "var/lib"), 0755); err != nil {
		t.Fatal(err)
	}
	e, err := bootstrap(host)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(host, "var/lib/homenode-install"))
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatal(info, err)
	}
	if other, err := bootstrap(host); err == nil {
		other.Close()
		t.Fatal("concurrent installer admitted")
	}
	e.Close()
	e, err = bootstrap(host)
	if err != nil {
		t.Fatal(err)
	}
	e.Close()
}

func TestBootstrapRejectsUnsafeDirectories(t *testing.T) {
	for _, which := range []string{"var", "lib", "journal", "symlink"} {
		t.Run(which, func(t *testing.T) {
			host := t.TempDir()
			base := filepath.Join(host, "var/lib")
			if err := os.MkdirAll(base, 0755); err != nil {
				t.Fatal(err)
			}
			journal := filepath.Join(base, "homenode-install")
			switch which {
			case "var":
				os.Chmod(filepath.Join(host, "var"), 0777)
			case "lib":
				os.Chmod(base, 0777)
			case "journal":
				os.Mkdir(journal, 0755)
			case "symlink":
				os.Symlink(t.TempDir(), journal)
			}
			if e, err := bootstrap(host); err == nil {
				e.Close()
				t.Fatal("unsafe path admitted")
			}
		})
	}
}
