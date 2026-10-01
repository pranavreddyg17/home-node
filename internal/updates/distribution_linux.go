//go:build linux

package updates

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// ValidateDistributionPackage uses the installed distribution decoder for xz
// support. It refuses root and writable descriptors, accepts no executable or
// filesystem path from the caller, and never extracts/installs. The maintenance
// caller must execute this in the dedicated constrained inspection worker;
// this API alone does not establish that isolation or install authority.
func ValidateDistributionPackage(ctx context.Context, file *os.File, release ReleaseMetadata) error {
	if os.Geteuid() == 0 || file == nil {
		return ErrPackageArchive
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !releaseName.MatchString(release.Release) || release.Platform != "ubuntu-24.04-amd64" {
		return ErrPackageControl
	}
	flags, err := unix.FcntlInt(file.Fd(), unix.F_GETFL, 0)
	if err != nil || flags&unix.O_ACCMODE != unix.O_RDONLY {
		return ErrPackageArchive
	}
	if _, err = InspectDebianArchive(file); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	for _, role := range []string{"control", "data"} {
		flag := "--ctrl-tarfile"
		if role == "data" {
			flag = "--fsys-tarfile"
		}
		command := exec.CommandContext(ctx, "/usr/bin/dpkg-deb", flag, "/proc/self/fd/3")
		command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		command.Cancel = func() error {
			err := unix.Kill(-command.Process.Pid, unix.SIGKILL)
			if errors.Is(err, unix.ESRCH) {
				return os.ErrProcessDone
			}
			return err
		}
		command.WaitDelay = 2 * time.Second
		command.ExtraFiles = []*os.File{file}
		command.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C"}
		command.Stderr = io.Discard
		pipe, err := command.StdoutPipe()
		if err != nil {
			return err
		}
		if err = command.Start(); err != nil {
			_ = pipe.Close()
			return err
		}
		if role == "control" {
			err = ValidateControlArchive(contextualReader{ctx, pipe}, release)
		} else {
			err = ValidatePayloadArchive(ctx, pipe)
		}
		if err != nil {
			cancel()
		}
		closeErr := pipe.Close()
		waitErr := command.Wait()
		if err = errors.Join(err, closeErr, waitErr, ctx.Err()); err != nil {
			return err
		}
	}
	return ctx.Err()
}
