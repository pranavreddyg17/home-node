package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// Hold each endpoint parent for the entire process lifetime. In particular,
// a second supervisor must not reconcile the journal or replace an endpoint.
func lockEndpointParents(paths []string) (map[string]*os.File, error) {
	parents := make(map[string]*os.File)
	fail := func(err error) (map[string]*os.File, error) {
		for _, file := range parents {
			_ = file.Close()
		}
		return nil, err
	}
	for _, path := range paths {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return fail(errors.New("invalid endpoint path"))
		}
		parent := filepath.Dir(path)
		if parents[parent] != nil {
			continue
		}
		fd, err := unix.Open(parent, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			return fail(err)
		}
		file := os.NewFile(uintptr(fd), parent)
		parents[parent] = file
		if err := qualifyEndpointParent(file); err != nil {
			return fail(err)
		}
		if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
			return fail(fmt.Errorf("endpoint parent already in use: %w", err))
		}
	}
	return parents, nil
}

func qualifyEndpointParent(file *os.File) error {
	info, err := file.Stat()
	if err != nil {
		return err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != 0 || !info.IsDir() || info.Mode().Perm()&0022 != 0 || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return errors.New("unprotected endpoint parent")
	}
	named, err := os.Lstat(file.Name())
	if err != nil || !os.SameFile(info, named) {
		return errors.New("endpoint parent replaced")
	}
	return nil
}

// Only an exact root-owned socket in a retained, non-writable parent may be
// retired, and only ECONNREFUSED proves that the old listener is absent.
func retireStaleEndpoint(parent *os.File, path, network string, gid int) error {
	if parent == nil || filepath.Dir(path) != parent.Name() {
		return errors.New("endpoint parent not retained")
	}
	if err := qualifyEndpointParent(parent); err != nil {
		return err
	}
	name := filepath.Base(path)
	var before unix.Stat_t
	err := unix.Fstatat(int(parent.Fd()), name, &before, unix.AT_SYMLINK_NOFOLLOW)
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	if err != nil {
		return err
	}
	if before.Mode&unix.S_IFMT != unix.S_IFSOCK || before.Uid != 0 || before.Gid != uint32(gid) || before.Nlink != 1 || before.Mode&07777 != 0660 {
		return errors.New("foreign endpoint preserved")
	}
	conn, err := net.DialTimeout(network, path, time.Second)
	if err == nil {
		_ = conn.Close()
		return errors.New("live endpoint preserved")
	}
	if !errors.Is(err, syscall.ECONNREFUSED) {
		return fmt.Errorf("endpoint liveness uncertain: %w", err)
	}
	if err := qualifyEndpointParent(parent); err != nil {
		return err
	}
	var after unix.Stat_t
	if err := unix.Fstatat(int(parent.Fd()), name, &after, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	if before != after {
		return errors.New("endpoint changed during observation")
	}
	return unix.Unlinkat(int(parent.Fd()), name, 0)
}
