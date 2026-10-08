package install

import (
	"os"
	"runtime"
	"testing"
)

// Only the explicitly enabled native account fixture may create the fixed
// journal needed by installed systemd conditions. Never adopt an existing one.
func installedAccountFixtureJournal(t *testing.T) string {
	t.Helper()
	if runtime.GOOS != "linux" || os.Geteuid() != 0 || os.Getenv("HOMENODE_ACCOUNT_INSPECTION_INTEGRATION") != "1" {
		t.Fatal("fixed account journal requires disposable Linux root fixture")
	}
	const path = "/var/lib/homenode-install"
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal("fixture refuses occupied installation journal", err)
	}
	original, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		defer root.Close()
		current, err := os.Lstat(path)
		if err != nil || !os.SameFile(original, current) {
			t.Error("fixture journal changed; preserving its contents", err)
			return
		}
		lock, err := root.Lstat("install.lock")
		if err != nil || !lock.Mode().IsRegular() || lock.Mode().Perm() != 0600 || !owned(lock, os.Geteuid()) || lock.Size() != 0 {
			t.Error("fixture lock content changed; preserving journal", err)
			return
		}
		// Remove only known files from this freshly created, retained directory.
		// Unexpected entries make directory removal fail and remain for inspection.
		for _, name := range []string{"install.lock", "accounts.json", "maintenance-accounts.json", "guest-identity-nss-intent.json", "guest-uid-intent.json", "guest-storage-intent.json"} {
			info, err := root.Lstat(name)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || !owned(info, os.Geteuid()) {
				t.Error("fixture journal entry changed; preserving it", name, err)
				continue
			}
			if err := root.Remove(name); err != nil {
				t.Error("fixture journal cleanup", name, err)
			}
		}
		current, err = os.Lstat(path)
		if err != nil || !os.SameFile(original, current) {
			t.Error("fixture journal pathname changed before directory cleanup", err)
			return
		}
		if err := os.Remove(path); err != nil {
			t.Error("fixture journal contains unexpected entries; preserving it", err)
		}
	})
	return path
}
