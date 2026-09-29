package install

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func roots(t *testing.T) (string, string) {
	t.Helper()
	host, jr := t.TempDir(), t.TempDir()
	if err := os.Chmod(host, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(jr, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"etc", "etc/systemd", "etc/systemd/system", "var", "var/lib"} {
		if err := os.Mkdir(filepath.Join(host, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	return host, jr
}
func fixturePlan() Plan {
	uid, gid := os.Geteuid(), os.Getegid()
	return Plan{Items: []Item{
		{Path: "etc/homenode", Directory: true, Mode: 0755, UID: uid, GID: gid},
		{Path: "etc/homenode/runtime-policy.json", Mode: 0640, UID: uid, GID: gid, Data: []byte("fixture policy bytes")},
		{Path: "var/lib/homenode", Directory: true, Mode: 0755, UID: uid, GID: gid},
		{Path: "var/lib/homenode/control", Directory: true, Mode: 0700, UID: uid, GID: gid},
	}}
}
func openEngine(t *testing.T, host, jr string) *Engine {
	t.Helper()
	engine, err := Open(host, jr)
	if err != nil {
		t.Fatal(err)
	}
	return engine
}

func TestInstallResumeRollbackAndLock(t *testing.T) {
	host, jr := roots(t)
	engine := openEngine(t, host, jr)
	ctx := context.Background()
	plan := fixturePlan()
	if err := engine.Apply(ctx, plan); err != nil {
		t.Fatal(err)
	}
	if other, err := Open(host, jr); err == nil {
		other.Close()
		t.Fatal("concurrent installer admitted")
	}
	if err := engine.Apply(ctx, plan); err != nil {
		t.Fatal("idempotent installation", err)
	}
	changed := fixturePlan()
	changed.Items[1].Data = []byte("another policy")
	if err := engine.Apply(ctx, changed); err != ErrConflict {
		t.Fatal("changed plan resumed", err)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	engine = openEngine(t, host, jr)
	defer engine.Close()
	if err := engine.Apply(ctx, plan); err != nil {
		t.Fatal("process recreation", err)
	}
	if err := engine.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(host, "etc/homenode")); !os.IsNotExist(err) {
		t.Fatal("created directory retained", err)
	}
	if _, err := os.Stat(filepath.Join(host, "etc/systemd/system")); err != nil {
		t.Fatal("OS directory removed", err)
	}
	if err := engine.Rollback(ctx); err != nil {
		t.Fatal("rollback replay", err)
	}
	if err := engine.Apply(ctx, plan); err != ErrConflict {
		t.Fatal("rolled-back plan resurrected", err)
	}
}

func TestInstallerPreservesForeignConfigurationAndOwnerData(t *testing.T) {
	t.Run("foreign file", func(t *testing.T) {
		host, jr := roots(t)
		os.Mkdir(filepath.Join(host, "etc/homenode"), 0755)
		file := filepath.Join(host, "etc/homenode/runtime-policy.json")
		os.WriteFile(file, []byte("owner configuration"), 0600)
		engine := openEngine(t, host, jr)
		defer engine.Close()
		if err := engine.Apply(context.Background(), fixturePlan()); !errors.Is(err, ErrConflict) {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(file)
		if string(data) != "owner configuration" {
			t.Fatal("foreign configuration overwritten")
		}
		if _, err := os.Stat(filepath.Join(jr, "install.json")); !os.IsNotExist(err) {
			t.Fatal("foreign ownership recorded", err)
		}
	})
	t.Run("changed file", func(t *testing.T) {
		host, jr := roots(t)
		engine := openEngine(t, host, jr)
		defer engine.Close()
		if err := engine.Apply(context.Background(), fixturePlan()); err != nil {
			t.Fatal(err)
		}
		file := filepath.Join(host, "etc/homenode/runtime-policy.json")
		if err := os.WriteFile(file, []byte("modified by owner"), 0640); err != nil {
			t.Fatal(err)
		}
		if err := engine.Rollback(context.Background()); !errors.Is(err, ErrConflict) {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(file)
		if string(data) != "modified by owner" {
			t.Fatal("changed configuration removed")
		}
	})
	t.Run("owner data", func(t *testing.T) {
		host, jr := roots(t)
		engine := openEngine(t, host, jr)
		defer engine.Close()
		if err := engine.Apply(context.Background(), fixturePlan()); err != nil {
			t.Fatal(err)
		}
		file := filepath.Join(host, "var/lib/homenode/control/owner-data")
		if err := os.WriteFile(file, []byte("preserve me"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := engine.Rollback(context.Background()); err == nil {
			t.Fatal("occupied data directory removed")
		}
		if data, err := os.ReadFile(file); err != nil || string(data) != "preserve me" {
			t.Fatal("owner content lost", err)
		}
	})
}

func TestInstallerRefusesEscapesAndUnsafePaths(t *testing.T) {
	host, jr := roots(t)
	engine := openEngine(t, host, jr)
	defer engine.Close()
	for _, name := range []string{"etc/shadow", "../etc/homenode/services.env", "etc/homenode/../shadow", "/etc/homenode/services.env"} {
		plan := fixturePlan()
		plan.Items[1].Path = name
		if err := engine.Apply(context.Background(), plan); err != ErrPlan {
			t.Fatal(name, err)
		}
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(host, "etc/homenode")); err != nil {
		t.Fatal(err)
	}
	if err := engine.Apply(context.Background(), fixturePlan()); !errors.Is(err, ErrConflict) {
		t.Fatal("symlink accepted", err)
	}
	if names, err := os.ReadDir(outside); err != nil || len(names) != 0 {
		t.Fatal("escaped installation", err)
	}
}

func TestConfigurationCrashHelper(t *testing.T) {
	if os.Getenv("HOMENODE_INSTALL_CRASH_HELPER") != "1" {
		return
	}
	engine, err := Open(os.Getenv("HOMENODE_INSTALL_HOST"), os.Getenv("HOMENODE_INSTALL_JOURNAL"))
	if err != nil {
		t.Fatal(err)
	}
	engine.checkpoint = func(stage, name string) error {
		if stage == os.Getenv("HOMENODE_INSTALL_STAGE") && name == "etc/homenode/runtime-policy.json" {
			os.Exit(77)
		}
		return nil
	}
	if os.Getenv("HOMENODE_INSTALL_STAGE") != "removed" {
		err = engine.Apply(context.Background(), fixturePlan())
	} else {
		err = engine.Rollback(context.Background())
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Fatal("crash checkpoint was not reached")
}

func crash(t *testing.T, host, jr, stage string) {
	t.Helper()
	command := exec.Command(os.Args[0], "-test.run=^TestConfigurationCrashHelper$")
	command.Env = append(os.Environ(), "HOMENODE_INSTALL_CRASH_HELPER=1", "HOMENODE_INSTALL_HOST="+host, "HOMENODE_INSTALL_JOURNAL="+jr, "HOMENODE_INSTALL_STAGE="+stage)
	output, err := command.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 77 {
		t.Fatalf("helper did not exit at crash point: %v %s", err, output)
	}
}

func TestProcessExitBetweenFilesystemAndJournalCommit(t *testing.T) {
	host, jr := roots(t)
	crash(t, host, jr, "published")
	engine := openEngine(t, host, jr)
	j, err := engine.load()
	if err != nil || j.Items[1].State != "pending" {
		t.Fatal("missing interrupted intent", j, err)
	}
	if err = engine.Apply(context.Background(), fixturePlan()); err != nil {
		t.Fatal("publish reconciliation", err)
	}
	engine.Close()
	crash(t, host, jr, "removed")
	engine = openEngine(t, host, jr)
	defer engine.Close()
	if err = engine.Rollback(context.Background()); err != nil {
		t.Fatal("rollback reconciliation", err)
	}
	if _, err = os.Stat(filepath.Join(host, "etc/homenode")); !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestBorrowedDirectoryAndCancelledInstall(t *testing.T) {
	host, jr := roots(t)
	if err := os.Mkdir(filepath.Join(host, "etc/homenode"), 0755); err != nil {
		t.Fatal(err)
	}
	engine := openEngine(t, host, jr)
	defer engine.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := engine.Apply(ctx, fixturePlan()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(jr, "install.json")); !os.IsNotExist(err) {
		t.Fatal("cancelled install wrote journal", err)
	}
	if err := engine.Apply(context.Background(), fixturePlan()); err != nil {
		t.Fatal(err)
	}
	if err := engine.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(host, "etc/homenode")); err != nil {
		t.Fatal("borrowed directory removed", err)
	}
}

func TestUnsafeAndCorruptJournalAreRefused(t *testing.T) {
	host, jr := roots(t)
	if err := os.Chmod(jr, 0755); err != nil {
		t.Fatal(err)
	}
	if engine, err := Open(host, jr); err == nil {
		engine.Close()
		t.Fatal("public journal accepted")
	}
	if err := os.Chmod(jr, 0700); err != nil {
		t.Fatal(err)
	}
	engine := openEngine(t, host, jr)
	defer engine.Close()
	if err := os.WriteFile(filepath.Join(jr, "install.json"), []byte(`{"version":999}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := engine.Apply(context.Background(), fixturePlan()); !errors.Is(err, ErrConflict) {
		t.Fatal("corrupt journal trusted", err)
	}
	if err := engine.Rollback(context.Background()); !errors.Is(err, ErrConflict) {
		t.Fatal("corrupt rollback admitted", err)
	}
	if _, err := os.Stat(filepath.Join(host, "etc/homenode")); !os.IsNotExist(err) {
		t.Fatal("corrupt journal changed filesystem", err)
	}
}

func TestProcessExitWithIncompleteStagingFile(t *testing.T) {
	host, jr := roots(t)
	crash(t, host, jr, "staged")
	engine := openEngine(t, host, jr)
	defer engine.Close()
	if err := engine.Apply(context.Background(), fixturePlan()); err != nil {
		t.Fatal("partial staging did not resume", err)
	}
	data, err := os.ReadFile(filepath.Join(host, "etc/homenode/runtime-policy.json"))
	if err != nil || string(data) != string(fixturePlan().Items[1].Data) {
		t.Fatal("partial configuration published", err)
	}
}

func TestRootProvisioningSeparatesControllerOwnership(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root fixture run; this does not activate host services")
	}
	host, jr := roots(t)
	engine := openEngine(t, host, jr)
	defer engine.Close()
	plan := fixturePlan()
	plan.Items[3].UID = 65534
	plan.Items[3].GID = 65534
	if err := engine.Apply(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(host, "var/lib/homenode/control"))
	if err != nil || !owned(info, 65534) || info.Mode().Perm() != 0700 {
		t.Fatal("controller data identity incorrect", err)
	}
	if err = engine.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
}
