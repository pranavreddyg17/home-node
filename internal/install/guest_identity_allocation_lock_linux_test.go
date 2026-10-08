//go:build linux

package install

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestGuestIdentityAllocationLockChild(t *testing.T) {
	path := os.Getenv("HOMENODE_ALLOCATION_LOCK_CHILD_PATH")
	if path == "" {
		t.Skip("allocation lock subprocess only")
	}
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_UPDATE_INIT_INTEGRATION") != "1" {
		t.Fatal("disposable Linux root fixture required")
	}
	file, err := os.OpenFile(path, os.O_RDWR|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	blocked := os.Getenv("HOMENODE_ALLOCATION_LOCK_CHILD_BLOCKED") == "1"
	for _, command := range []int{unix.F_SETLK, unix.F_OFD_SETLK} {
		lock := unix.Flock_t{Type: unix.F_WRLCK, Whence: io.SeekStart, Len: 0}
		err := unix.FcntlFlock(file.Fd(), command, &lock)
		if blocked {
			if !errors.Is(err, unix.EAGAIN) && !errors.Is(err, unix.EACCES) {
				t.Fatal("parent lock did not exclude competing record lock", command, err)
			}
		} else {
			if err != nil {
				t.Fatal("closed transaction retained account lock", command, err)
			}
			lock.Type = unix.F_UNLCK
			if err := unix.FcntlFlock(file.Fd(), command, &lock); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestRootGuestIdentityAllocationLockInteroperability(t *testing.T) {
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_UPDATE_INIT_INTEGRATION") != "1" {
		t.Skip("explicit disposable Linux root fixture")
	}
	path := t.TempDir()
	directory, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	probe := func(blocked bool) error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestGuestIdentityAllocationLockChild$", "-test.count=1")
		value := "0"
		if blocked {
			value = "1"
		}
		command.Env = append(os.Environ(), "HOMENODE_ALLOCATION_LOCK_CHILD_PATH="+filepath.Join(path, ".pwd.lock"), "HOMENODE_ALLOCATION_LOCK_CHILD_BLOCKED="+value)
		output, err := command.CombinedOutput()
		if err != nil {
			t.Errorf("record lock probe failed: %s", output)
		}
		return err
	}
	err = withGuestIdentityAllocationLock(context.Background(), directory, func(ctx context.Context, check func() error) error {
		if err := probe(true); err != nil {
			return err
		}
		// Closing another descriptor must not release the OFD-held lock.
		other, err := directory.Open(".pwd.lock")
		if err != nil {
			return err
		}
		if err := other.Close(); err != nil {
			return err
		}
		if err := check(); err != nil {
			return err
		}
		return probe(true)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := probe(false); err != nil {
		t.Fatal(err)
	}
	if _, err := directory.Stat(".pwd.lock"); err != nil {
		t.Fatal("transaction removed shared lock file", err)
	}
}
