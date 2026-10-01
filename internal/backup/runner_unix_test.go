//go:build linux || darwin

package backup

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestMaintenanceRunnerExclusiveAndPersistentInode(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	first, err := lockMaintenanceRunner(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	before, err := first.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if other, err := lockMaintenanceRunner(context.Background(), dir); !errors.Is(err, ErrMaintenanceRunner) {
		if other != nil {
			other.Close()
		}
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	next, err := lockMaintenanceRunner(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	after, err := next.Stat()
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("lock inode replaced", err)
	}
}

func TestMaintenanceRunnerRejectsUnsafeFiles(t *testing.T) {
	for _, kind := range []string{"symlink", "hardlink", "mode", "content", "fifo", "directory", "parent"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Chmod(dir, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "maintenance.lock")
			var err error
			switch kind {
			case "symlink":
				err = os.Symlink("target", path)
			case "hardlink":
				err = os.WriteFile(filepath.Join(dir, "target"), nil, 0600)
				if err == nil {
					err = os.Link(filepath.Join(dir, "target"), path)
				}
			case "mode":
				err = os.WriteFile(path, nil, 0644)
			case "content":
				err = os.WriteFile(path, []byte("stale"), 0600)
			case "fifo":
				err = unix.Mkfifo(path, 0600)
			case "directory":
				err = os.Mkdir(path, 0700)
			case "parent":
				err = os.Chmod(dir, 0755)
			}
			if err != nil {
				t.Fatal(err)
			}
			file, err := lockMaintenanceRunner(context.Background(), dir)
			if file != nil {
				file.Close()
			}
			if err == nil {
				t.Fatal("unsafe runner lock accepted")
			}
		})
	}
}

func TestMaintenanceRunnerCanceledClaim(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dir := t.TempDir()
	file, err := lockMaintenanceRunner(ctx, dir)
	if file != nil {
		file.Close()
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err = os.Lstat(filepath.Join(dir, "maintenance.lock")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("canceled claim created lock", err)
	}
}

func TestMaintenanceRunnerProcessDeath(t *testing.T) {
	if dir := os.Getenv("HOMENODE_RUNNER_TEST_CHILD"); dir != "" {
		file, err := lockMaintenanceRunner(context.Background(), dir)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if _, err = os.Stdout.Write([]byte{1}); err != nil {
			t.Fatal(err)
		}
		var b [1]byte
		_, _ = os.Stdin.Read(b[:])
		return
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	childContext, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(childContext, os.Args[0], "-test.run=^TestMaintenanceRunnerProcessDeath$")
	command.Env = append(os.Environ(), "HOMENODE_RUNNER_TEST_CHILD="+dir)
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	input, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = command.Process.Kill(); _ = command.Wait() }()
	var ready [1]byte
	if _, err = output.Read(ready[:]); err != nil || ready[0] != 1 {
		t.Fatal("child did not acquire runner", err)
	}
	if file, err := lockMaintenanceRunner(context.Background(), dir); !errors.Is(err, ErrMaintenanceRunner) {
		if file != nil {
			file.Close()
		}
		t.Fatal("live child lost ownership", err)
	}
	if err = command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err = command.Wait(); err == nil {
		t.Fatal("child unexpectedly exited normally")
	}
	file, err := lockMaintenanceRunner(context.Background(), dir)
	if err != nil {
		t.Fatal("kernel retained dead runner", err)
	}
	file.Close()
}
