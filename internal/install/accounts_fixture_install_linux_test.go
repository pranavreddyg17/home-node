//go:build linux

package install

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Install real generated units, using the fixture's provisioned account IDs.
// Catalog payloads are test data; no workload is launched or image qualified.
func installAccountFixtureConfiguration(t *testing.T, e *Engine, accounts Accounts, maintenance MaintenanceAccount) {
	t.Helper()
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_ACCOUNT_INSPECTION_INTEGRATION") != "1" || e.host.Name() != "/" {
		t.Fatal("native installation fixture requires disposable Linux root")
	}
	ctx := context.Background()
	if err := e.requireRecoveryJournalLocation(ctx); err != nil {
		t.Fatal(err)
	}
	c, _, _, now := configurationFixture(t)
	c.Accounts = accounts
	c.Policy.ControllerUID, c.Policy.TransferUID = accounts.ControllerUID, accounts.TransferUID
	c.Maintenance = &maintenance
	c.BackupRepositoryID = strings.Repeat("a", 64)
	c.BackupRelease, c.BackupDriveUUID = "0.1.0", "abcd-1234"
	preview, err := ConfigurationPlan(c, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Apply(ctx, preview.Plan); err != nil {
		t.Fatal("native owned configuration installation", err)
	}
	installed, err := e.load()
	if err != nil {
		t.Fatal(err)
	}
	command := func(args ...string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "/usr/bin/systemctl", args...)
		cmd.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C", "SYSTEMD_PAGER=cat"}
		cmd.WaitDelay = time.Second
		return errors.Join(cmd.Run(), ctx.Err())
	}
	sliceStarted := false
	t.Cleanup(func() {
		// The main fixture's defer releases its engine before cleanups execute.
		cleanup, err := Open("/", "/var/lib/homenode-install")
		if err != nil {
			t.Error("native installation cleanup admission", err)
			return
		}
		defer cleanup.Close()
		current, err := cleanup.load()
		if err != nil || current.ID != installed.ID || current.Digest != installed.Digest {
			t.Error("native installation authority changed; preserving state", err)
			return
		}
		if _, err := cleanup.journalRoot.Lstat("guest-identity-nss-stage.json"); !os.IsNotExist(err) {
			t.Error("NSS restoration incomplete; preserving configuration and marker", err)
			return
		}
		if _, err := cleanup.journalRoot.Lstat("guest-uid-allocation-stage.json"); !os.IsNotExist(err) {
			t.Error("allocator restoration incomplete; preserving configuration and marker", err)
			return
		}
		if sliceStarted {
			if err := command("stop", "homenode.slice"); err != nil {
				t.Error("fixture slice cleanup", err)
				return
			}
		}
		if err := cleanup.Rollback(context.Background()); err != nil {
			t.Error("native configuration rollback; preserving journal and marker", err)
			return
		}
		if err := command("daemon-reload"); err != nil {
			t.Error("native configuration cleanup reload; preserving marker", err)
			return
		}
		if err := cleanup.journalRoot.Remove("install.json"); err != nil {
			t.Error("fixture installation record cleanup", err)
			return
		}
		if err := cleanup.journalRoot.Remove("recovery-blocked"); err != nil && !os.IsNotExist(err) {
			t.Error("fixture activation marker cleanup", err)
		}
	})
	if err := e.blockRecoveryActivation(ctx); err != nil {
		t.Fatal("native fixture activation barrier", err)
	}
	if err := command("daemon-reload"); err != nil {
		t.Fatal(err)
	}
	if err := command("start", "homenode.slice"); err != nil {
		t.Fatal(err)
	}
	sliceStarted = true
	if err := e.QuiesceRecovery(ctx); err != nil {
		t.Fatal("native owned activation exclusion qualification", err)
	}
}
