//go:build linux

package backup

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"time"

	"golang.org/x/sys/unix"
)

// QualifyExt4Disk checks a pinned read-only disk while its caller retains an
// exclusive stopped-runtime lease. It never repairs a source filesystem. This
// does not establish application consistency or prove the absence of other
// writers; those remain obligations of the maintenance coordinator.
func QualifyExt4Disk(ctx context.Context, source *os.File) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if source == nil {
		return ErrManifest
	}
	info, err := source.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() < 16<<20 || info.Size() > 512<<30 {
		return ErrManifest
	}
	flags, err := unix.FcntlInt(source.Fd(), unix.F_GETFL, 0)
	if err != nil || flags&unix.O_ACCMODE != unix.O_RDONLY || requireCleanExt4Header(source) != nil {
		return ErrManifest
	}
	deadline, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	command := exec.CommandContext(deadline, "/usr/sbin/e2fsck", "-f", "-n", "/proc/self/fd/3")
	command.Env = []string{"LANG=C", "LC_ALL=C", "PATH=/usr/sbin:/usr/bin"}
	command.ExtraFiles = []*os.File{source}
	command.WaitDelay = 2 * time.Second
	// Checker diagnostics may contain guest-controlled names; discard them.
	if err = command.Run(); err != nil {
		return errors.Join(ErrManifest, deadline.Err())
	}
	after, err := source.Stat()
	if err != nil || !os.SameFile(info, after) || info.Size() != after.Size() || !info.ModTime().Equal(after.ModTime()) || requireCleanExt4Header(source) != nil {
		return ErrManifest
	}
	return deadline.Err()
}
