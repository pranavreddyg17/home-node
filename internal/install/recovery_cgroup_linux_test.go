//go:build linux

package install

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Only disposable Linux CI may create these fixed product-path observations.
// An existing unit or cgroup is never reused or replaced.
func TestNativeRecoveryGuestCgroupObservation(t *testing.T) {
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_RECOVERY_CGROUP_INTEGRATION") != "1" {
		t.Skip("explicit disposable Linux root fixture")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ObserveRecoveryGuestsEmpty(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled kernel observation ignored", err)
	}
	const slice = "homenode.slice"
	const service = "homenode-recovery-cgroup-fixture.service"
	const slicePath = "/run/systemd/system/homenode.slice"
	const servicePath = "/run/systemd/system/homenode-recovery-cgroup-fixture.service"
	for _, path := range []string{slicePath, servicePath, "/etc/systemd/system/homenode.slice", "/usr/lib/systemd/system/homenode.slice", "/sys/fs/cgroup/homenode.slice"} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("fixture path occupied", path, err)
		}
	}
	if err := ObserveRecoveryGuestsEmpty(context.Background()); !errors.Is(err, ErrConflict) {
		t.Fatal("absent hierarchy treated as empty", err)
	}
	command := func(args ...string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "/usr/bin/systemctl", args...)
		cmd.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C", "SYSTEMD_PAGER=cat"}
		cmd.WaitDelay = time.Second
		return cmd.Run()
	}
	write := func(path, text string) error {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		_, writeErr := file.WriteString(text)
		return errors.Join(writeErr, file.Close())
	}
	if err := write(slicePath, "[Unit]\nDescription=Disposable recovery guest slice\n[Slice]\nTasksMax=16\nMemoryMax=64M\n"); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "recovery-blocked")
	if err := os.WriteFile(marker, nil, 0600); err != nil {
		t.Fatal(err)
	}
	serviceCreated := false
	defer func() {
		// Report cleanup errors as test failures; never hide leftover fixture state.
		if serviceCreated {
			if err := command("stop", service); err != nil {
				t.Errorf("stop fixture service: %v", err)
			}
		}
		if err := command("stop", slice); err != nil {
			t.Errorf("stop fixture slice: %v", err)
		}
		if serviceCreated {
			if err := os.Remove(servicePath); err != nil {
				t.Errorf("remove fixture service: %v", err)
			}
		}
		if err := os.Remove(slicePath); err != nil {
			t.Errorf("remove fixture slice: %v", err)
		}
		if err := command("daemon-reload"); err != nil {
			t.Errorf("reload after fixture cleanup: %v", err)
		}
	}()
	if err := write(servicePath, "[Unit]\nDescription=Disposable populated guest cgroup\nConditionPathExists=!"+marker+"\n[Service]\nType=simple\nSlice=homenode.slice\nExecStart=/bin/sleep 60\nKillMode=control-group\n"); err != nil {
		t.Fatal(err)
	}
	serviceCreated = true
	if err := command("daemon-reload"); err != nil {
		t.Fatal(err)
	}
	// Read the real manager's typed property, rather than substituting a JSON
	// fixture. Use only this owned disposable unit and temporary marker.
	observeGuard := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "/usr/bin/busctl", "--system", "--no-pager", "--json=short", "--auto-start=no", "--allow-interactive-authorization=no", "--timeout=4", "get-property", "org.freedesktop.systemd1", "/org/freedesktop/systemd1/unit/homenode_2drecovery_2dcgroup_2dfixture_2eservice", "org.freedesktop.systemd1.Unit", "Conditions")
		cmd.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C", "SYSTEMD_PAGER=cat"}
		cmd.WaitDelay = time.Second
		output := &activationConditionsOutput{}
		cmd.Stdout = output
		if err := cmd.Run(); err != nil {
			return errors.Join(err, ctx.Err())
		}
		return validateActivationConditionsAtPath(output.Bytes(), marker)
	}
	if err := observeGuard(); err != nil {
		t.Fatal("manager-loaded guard refused", err)
	}
	if err := command("start", slice); err != nil {
		t.Fatal(err)
	}
	if err := ObserveRecoveryGuestsEmpty(context.Background()); err != nil {
		t.Fatal("empty kernel hierarchy refused", err)
	}
	if err := command("start", service); err != nil {
		t.Fatal(err)
	}
	if err := observeGuard(); err != nil {
		t.Fatal("loaded guard after refused activation", err)
	}
	if err := ObserveRecoveryGuestsEmpty(context.Background()); err != nil {
		t.Fatal("activation marker did not preserve empty hierarchy", err)
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	if err := command("start", service); err != nil {
		t.Fatal(err)
	}
	if err := ObserveRecoveryGuestsEmpty(context.Background()); !errors.Is(err, ErrConflict) {
		t.Fatal("populated descendant hierarchy admitted", err)
	}
	if err := os.WriteFile(marker, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := observeGuard(); err != nil {
		t.Fatal("active service guard observation", err)
	}
	if err := ObserveRecoveryGuestsEmpty(context.Background()); !errors.Is(err, ErrConflict) {
		t.Fatal("marker mistaken for active guest teardown", err)
	}
	if err := command("stop", service); err != nil {
		t.Fatal(err)
	}
	if err := ObserveRecoveryGuestsEmpty(context.Background()); err != nil {
		t.Fatal("stopped descendant hierarchy refused", err)
	}
}
