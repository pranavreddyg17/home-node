//go:build linux

package guestinit

import (
	"golang.org/x/sys/unix"
	"os"
)

func publishIntent(root *os.Root, source string) error {
	parent, err := root.Open(".")
	if err != nil {
		return ErrDirectory
	}
	defer parent.Close()
	fd := int(parent.Fd())
	if unix.Renameat2(fd, source, fd, intentName, unix.RENAME_NOREPLACE) != nil {
		return ErrDirectory
	}
	if parent.Sync() != nil {
		return ErrDirectory
	}
	return nil
}
