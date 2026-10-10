//go:build linux

package main

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func qualifyEndpointParentACL(file *os.File) error {
	for _, name := range []string{"system.posix_acl_access", "system.posix_acl_default"} {
		if _, err := unix.Fgetxattr(int(file.Fd()), name, nil); !errors.Is(err, unix.ENODATA) {
			return errors.New("endpoint parent ACL unavailable or present")
		}
	}
	return nil
}
