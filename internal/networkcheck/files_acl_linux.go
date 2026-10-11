package networkcheck

import (
	"errors"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// Mode bits do not identify named users admitted by an extended POSIX ACL.
// Require absence, including refusing filesystems that cannot establish it.
func protectedPEMACL(file *os.File) error {
	_, err := unix.Fgetxattr(int(file.Fd()), "system.posix_acl_access", nil)
	if !errors.Is(err, unix.ENODATA) {
		return ErrIdentity
	}
	return nil
}

func tlsChangeTimeUnchanged(before, after *syscall.Stat_t) bool {
	return before.Ctim == after.Ctim
}
