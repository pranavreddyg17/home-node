//go:build linux

package updates

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestVerificationSessionRefusesMissingOrInvalidPersistedRoot(t *testing.T) {
	for _, scenario := range []string{"missing", "invalid", "linked"} {
		t.Run(scenario, func(t *testing.T) {
			provisioned, directory := updateCacheFixture(t)
			name := filepath.Join(directory, "metadata", "root.json")
			switch scenario {
			case "invalid":
				if err := os.WriteFile(name, []byte("invalid retained root"), 0644); err != nil {
					t.Fatal(err)
				}
			case "linked":
				if err := os.Symlink("elsewhere", name); err != nil {
					t.Fatal(err)
				}
			}
			session, err := newVerificationSession(context.Background(), provisioned, "https://example.com/metadata")
			if session != nil {
				session.Close()
			}
			if err == nil {
				t.Fatal("untrusted root initialized a session")
			}
			// Failed initialization must release ownership, without replacing the root
			// with an older bundled or downloaded root or adopting linked state.
			lock, err := lockMetadataCache(context.Background(), provisioned)
			if err != nil {
				t.Fatal("failed session retained the lock", err)
			}
			lock.Close()
			switch scenario {
			case "missing":
				if _, err = os.Lstat(name); !os.IsNotExist(err) {
					t.Fatal("missing root silently provisioned", err)
				}
			case "invalid":
				data, err := os.ReadFile(name)
				if err != nil || string(data) != "invalid retained root" {
					t.Fatal("invalid persisted root replaced", err)
				}
			case "linked":
				target, err := os.Readlink(name)
				if err != nil || target != "elsewhere" {
					t.Fatal("linked root adopted", err)
				}
			}
		})
	}
}
