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
	attributes := []string{"system.posix_acl_access"}
	info, err := file.Stat()
	if err != nil {
		return ErrIdentity
	}
	if info.IsDir() {
		attributes = append(attributes, "system.posix_acl_default")
	}
	for _, attribute := range attributes {
		_, err := unix.Fgetxattr(int(file.Fd()), attribute, nil)
		if !errors.Is(err, unix.ENODATA) {
			return ErrIdentity
		}
	}
	return nil
}

func tlsChangeTimeUnchanged(before, after *syscall.Stat_t) bool {
	return before.Ctim == after.Ctim
}
