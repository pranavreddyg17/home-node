//go:build linux

package supervisor

import (
	"context"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// observeGuestMemoryDomain verifies a snapshot, not an activation barrier.
// It never substitutes the aggregate workload slice for a domain-specific limit.
func observeGuestMemoryDomain(ctx context.Context, relative, id string, maximum int64) (result error) {
	scope, err := guestCgroupScope(relative, id)
	if err != nil || maximum <= 0 {
		return ErrPolicy
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	fd, err := unix.Openat2(unix.AT_FDCWD, "/sys/fs/cgroup", &unix.OpenHow{Flags: unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC, Resolve: unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS})
	if err != nil {
		return err
	}
	descriptors := []int{fd}
	defer func() {
		for i := len(descriptors) - 1; i >= 0; i-- {
			result = errors.Join(result, unix.Close(descriptors[i]))
		}
	}()
	var filesystem unix.Statfs_t
	if unix.Fstatfs(fd, &filesystem) != nil || filesystem.Type != unix.CGROUP2_SUPER_MAGIC {
		return ErrPolicy
	}
	components := strings.Split(strings.TrimPrefix(relative, "/"), "/")
	snapshots := make([]unix.Stat_t, 0, len(components))
	for _, component := range components {
		if err := ctx.Err(); err != nil {
			return err
		}
		child, err := unix.Openat(descriptors[len(descriptors)-1], component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if err != nil {
			return err
		}
		descriptors = append(descriptors, child)
		var stat unix.Stat_t
		if unix.Fstat(child, &stat) != nil || stat.Uid != 0 || stat.Mode&0022 != 0 {
			return ErrPolicy
		}
		snapshots = append(snapshots, stat)
	}
	read := func(directory int, name string) (value string, result error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		fd, err := unix.Openat(directory, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
		if err != nil {
			return "", err
		}
		file := os.NewFile(uintptr(fd), name)
		defer func() { result = errors.Join(result, file.Close()) }()
		var stat unix.Stat_t
		if unix.Fstat(fd, &stat) != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Uid != 0 || stat.Mode&0022 != 0 {
			return "", ErrPolicy
		}
		data, err := io.ReadAll(io.LimitReader(file, 129))
		if err != nil {
			return "", err
		}
		if len(data) > 128 {
			return "", ErrPolicy
		}
		return strings.TrimSpace(string(data)), ctx.Err()
	}
	// The scope is component index1. Never climb into homenode.slice.
	selected := -1
	types := map[int]string{}
	for i := len(components) - 1; i >= 1; i-- {
		kind, err := read(descriptors[i+1], "cgroup.type")
		if err != nil {
			return err
		}
		types[i] = kind
		if kind == "threaded" {
			continue
		}
		if kind != "domain" && kind != "domain threaded" {
			return ErrPolicy
		}
		selected = i
		break
	}
	if selected < 1 || !strings.HasPrefix(relative, scope) {
		return ErrPolicy
	}
	limit, err := read(descriptors[selected+1], "memory.max")
	if err != nil {
		return err
	}
	numeric, err := strconv.ParseInt(limit, 10, 64)
	if err != nil || numeric <= 0 || numeric > maximum {
		return ErrPolicy
	}
	// Recheck every retained path component, type and bound after observation.
	for i, component := range components {
		var current unix.Stat_t
		if unix.Fstatat(descriptors[i], component, &current, unix.AT_SYMLINK_NOFOLLOW) != nil || current.Dev != snapshots[i].Dev || current.Ino != snapshots[i].Ino || current.Mode != snapshots[i].Mode || current.Uid != snapshots[i].Uid || current.Gid != snapshots[i].Gid {
			return ErrPolicy
		}
	}
	for i, kind := range types {
		current, err := read(descriptors[i+1], "cgroup.type")
		if err != nil {
			return err
		}
		if current != kind {
			return ErrPolicy
		}
	}
	current, err := read(descriptors[selected+1], "memory.max")
	if err != nil {
		return err
	}
	if current != limit {
		return ErrPolicy
	}
	return ctx.Err()
}
