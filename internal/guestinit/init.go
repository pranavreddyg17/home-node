// Package guestinit prepares only the private object directory on an admitted guest volume.
package guestinit

import (
	"errors"
	"os"
	"syscall"
)

var ErrDirectory = errors.New("guest object directory initialization failed")

// Prepare requires trusted exclusive startup. The caller must first admit the mount.
// Existing directories are checked, never repaired or recursively changed.
func Prepare(path string) error {
	if os.Geteuid() != 0 {
		return ErrDirectory
	}
	before, err := os.Lstat(path)
	if err != nil || !before.IsDir() || before.Mode().Perm() != 0755 {
		return ErrDirectory
	}
	owner, ok := before.Sys().(*syscall.Stat_t)
	if !ok || owner.Uid != 0 || owner.Gid != 0 {
		return ErrDirectory
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return ErrDirectory
	}
	defer root.Close()
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(before, opened) {
		return ErrDirectory
	}
	created := false
	if err := root.Mkdir("objects", 0700); err == nil {
		created = true
	} else if !errors.Is(err, os.ErrExist) {
		return ErrDirectory
	}
	directory, err := root.OpenFile("objects", os.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return ErrDirectory
	}
	defer directory.Close()
	info, err := directory.Stat()
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return ErrDirectory
	}
	identity, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return ErrDirectory
	}
	if created {
		if identity.Uid != 0 || identity.Gid != uint32(os.Getegid()) {
			return ErrDirectory
		}
		if directory.Chown(900, 900) != nil {
			return ErrDirectory
		}
	} else if identity.Uid != 900 || identity.Gid != 900 {
		return ErrDirectory
	}
	if directory.Sync() != nil {
		return ErrDirectory
	}
	parent, err := root.Open(".")
	if err != nil {
		return ErrDirectory
	}
	defer parent.Close()
	if parent.Sync() != nil {
		return ErrDirectory
	}
	return nil
}
