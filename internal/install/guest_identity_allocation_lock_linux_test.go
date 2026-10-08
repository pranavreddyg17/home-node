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

// Load libc before hiding /etc, then isolate all mount effects in the child.
// The real libc API uses /etc/.pwd.lock; bind only the owned temporary directory
// over that path's parent in a private mount namespace. No runner account file
// or lock is changed. A failed namespace setup refuses the fixture.
// Protocol reference: glibc-2.39/nss/lckpwdf.c in the glibc source repository.
const guestIdentityLibcLockProbe = `
import ctypes, errno, os, sys
libc = ctypes.CDLL("libc.so.6", use_errno=True)
libc.unshare.argtypes = [ctypes.c_int]
libc.mount.argtypes = [ctypes.c_char_p, ctypes.c_char_p, ctypes.c_char_p, ctypes.c_ulong, ctypes.c_void_p]
libc.lckpwdf.argtypes = []
libc.ulckpwdf.argtypes = []
if libc.unshare(0x00020000) != 0:
    raise OSError(ctypes.get_errno(), "private mount namespace required")
if libc.mount(None, b"/", None, (1 << 18) | 16384, None) != 0:
    raise OSError(ctypes.get_errno(), "private mount propagation required")
if libc.mount(os.fsencode(sys.argv[1]), b"/etc", None, 4096, None) != 0:
    raise OSError(ctypes.get_errno(), "owned fixture bind mount required")
ctypes.set_errno(0)
result = libc.lckpwdf()
error = ctypes.get_errno()
if sys.argv[2] == "1":
    if result != -1 or error != errno.EINTR:
        if result == 0:
            libc.ulckpwdf()
        raise RuntimeError("libc account writer was not excluded", result, error)
else:
    if result != 0:
        raise RuntimeError("released account lock remained unavailable", result, error)
    if libc.ulckpwdf() != 0:
        raise RuntimeError("libc account unlock failed")
`

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
	probeLibc := func(blocked bool) error {
		// libc bounds lock acquisition with its own fifteen-second alarm.
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		value := "0"
		if blocked {
			value = "1"
		}
		command := exec.CommandContext(ctx, "/usr/bin/python3", "-I", "-c", guestIdentityLibcLockProbe, path, value)
		command.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C"}
		command.WaitDelay = time.Second
		output, err := command.CombinedOutput()
		if err != nil {
			t.Errorf("isolated libc account lock probe failed: %s", output)
		}
		return errors.Join(err, ctx.Err())
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
		if err := probe(true); err != nil {
			return err
		}
		if err := probeLibc(true); err != nil {
			return err
		}
		return check()
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := probe(false); err != nil {
		t.Fatal(err)
	}
	if err := probeLibc(false); err != nil {
		t.Fatal(err)
	}
	if _, err := directory.Stat(".pwd.lock"); err != nil {
		t.Fatal("transaction removed shared lock file", err)
	}
}
