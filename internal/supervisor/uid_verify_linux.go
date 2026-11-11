//go:build linux

package supervisor

import (
	"context"
	"errors"
	"io"
	"os"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// Credential-only observation is also used for non-QEMU device probes.
func verifyGuestDACProcess(ctx context.Context, pid int, uid, gid uint32) error {
	return verifyReservedGuestProcess(ctx, pid, uid, gid, false)
}

func verifyQEMUProcess(ctx context.Context, pid int, uid, gid uint32) error {
	return verifyReservedGuestProcess(ctx, pid, uid, gid, true)
}

func verifyReservedGuestProcess(ctx context.Context, pid int, uid, gid uint32, qemu bool) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if os.Geteuid() != 0 || pid < 1 {
		return ErrPolicy
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	root, err := os.OpenRoot("/proc")
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, root.Close()) }()
	directory, err := root.Open(".")
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, directory.Close()) }()
	var fs unix.Statfs_t
	if unix.Fstatfs(int(directory.Fd()), &fs) != nil || fs.Type != unix.PROC_SUPER_MAGIC {
		return ErrPolicy
	}
	if err = qualifyGuestUIDNamespace(root); err != nil {
		return err
	}
	// A pidfd detects exit throughout this observation and prevents accepting
	// a replacement process merely because its numeric PID was reused.
	pidfd, err := unix.PidfdOpen(pid, 0)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, unix.Close(pidfd)) }()
	if err := checkLivePIDFD(ctx, pidfd); err != nil {
		return err
	}
	process, err := root.OpenRoot(strconv.Itoa(pid))
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, process.Close()) }()
	if qemu {
		cmdline, err := process.OpenFile("cmdline", os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		if err != nil {
			return err
		}
		arguments, readErr := io.ReadAll(io.LimitReader(cmdline, (128<<10)+1))
		closeErr := cmdline.Close()
		if err := errors.Join(readErr, closeErr); err != nil {
			return err
		}
		if err := validateQEMUSandboxArguments(arguments); err != nil {
			return err
		}
	}
	count, verified := 0, 0
	err = withGuestUIDTaskStatus(ctx, root, strconv.Itoa(pid), &count, func(status []byte) error {
		if err := validateGuestDACCredentials(status, uid, gid); err != nil {
			return err
		}
		if qemu {
			if err := validateQEMUSeccompStatus(status); err != nil {
				return err
			}
		}
		verified++
		return nil
	})
	if err != nil {
		return err
	}
	if verified == 0 {
		return ErrPolicy
	}
	return checkLivePIDFD(ctx, pidfd)
}

func checkLivePIDFD(ctx context.Context, pidfd int) error {
	events := []unix.PollFd{{Fd: int32(pidfd), Events: unix.POLLIN}}
	n, err := unix.Poll(events, 0)
	if err != nil {
		return err
	}
	if n != 0 {
		return ErrPolicy
	}
	return ctx.Err()
}
