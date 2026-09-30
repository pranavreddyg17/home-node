// Package guestinit prepares only the private object directory on an admitted guest volume.
package guestinit

import (
	"errors"
	"io"
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
	intent, err := admitIntent(root)
	if err != nil {
		return ErrDirectory
	}
	if !intent {
		if _, err := root.Lstat("objects"); errors.Is(err, os.ErrNotExist) {
			if err := createIntent(root); err != nil {
				return ErrDirectory
			}
			intent = true
		} else if err != nil {
			return ErrDirectory
		}
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
	if created || (intent && identity.Uid == 0 && identity.Gid == 0) {
		if identity.Uid != 0 || identity.Gid != uint32(os.Getegid()) {
			return ErrDirectory
		}
		entries, err := directory.ReadDir(1)
		if err != io.EOF || len(entries) != 0 {
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

const intentName = ".homenode-object-init"
const intentBytes = "homenode-object-init-v1\n"

func admitIntent(root *os.Root) (bool, error) {
	f, err := root.OpenFile(intentName, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, ErrDirectory
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 || info.Size() != int64(len(intentBytes)) {
		return false, ErrDirectory
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !ok || owner.Uid != 0 || owner.Gid != 0 || owner.Nlink != 1 {
		return false, ErrDirectory
	}
	data, err := io.ReadAll(io.LimitReader(f, 64))
	if err != nil || string(data) != intentBytes {
		return false, ErrDirectory
	}
	return true, nil
}

func createIntent(root *os.Root) error {
	f, err := root.OpenFile(intentName, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return ErrDirectory
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return ErrDirectory
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !ok || owner.Uid != 0 || owner.Gid != 0 || owner.Nlink != 1 || info.Mode().Perm() != 0600 {
		f.Close()
		return ErrDirectory
	}
	n, writeErr := io.WriteString(f, intentBytes)
	if n != len(intentBytes) && writeErr == nil {
		writeErr = io.ErrShortWrite
	}
	syncErr := f.Sync()
	closeErr := f.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
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
