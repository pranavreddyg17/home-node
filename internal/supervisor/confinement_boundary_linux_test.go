//go:build linux

package supervisor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Disposable per-domain static policy experiment. Never edits TEMPLATE or the
// distro abstraction; uncertain guest exit retains the profile and its source.
func nativeInstallWriteBoundary(t *testing.T, ctx context.Context, d *Domain) {
	t.Helper()
	if os.Getenv("HOMENODE_CONFINEMENT_WRITE_BOUNDARY") != "1" {
		return
	}
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_KVM_DOMAIN_INTEGRATION") != "1" {
		t.Fatal("disposable write-boundary opt-in required")
	}
	d.ConfinedWrites = true
	source, err := d.AppArmorWritePolicy()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("/etc/apparmor.d/libvirt", d.AppArmorName())
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(source); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := command(ctx, "", "/usr/sbin/apparmor_parser", "--add", path); err != nil {
		t.Fatal("per-domain policy load", err)
	}
	t.Cleanup(func() {
		scope, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		running, err := (LinuxBackend{}).Running(scope, d.ID)
		if err != nil || running {
			t.Error("retain uncertain write-boundary policy", err)
			return
		}
		observed, err := ObserveGuestUIDProcessConflicts(scope, GuestUIDPool{First: d.GuestUID, Last: d.GuestUID})
		if err != nil || observed.Blocked[d.GuestUID] {
			t.Error("retain policy with uncertain UID exit", err)
			return
		}
		if _, err := command(scope, "", "/usr/sbin/apparmor_parser", "--remove", path); err != nil {
			t.Error("policy retirement", err)
			return
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Error(err)
		}
	})
}
