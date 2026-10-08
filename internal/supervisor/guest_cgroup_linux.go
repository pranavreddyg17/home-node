//go:build linux

package supervisor

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// observeGuestMemoryDomain verifies a snapshot, not an activation barrier.
// It never substitutes the aggregate workload slice for a domain-specific limit.
func observeGuestMemoryDomain(ctx context.Context, relative, id string, maximum int64) (result error) {
	return observeGuestMemoryDomainAt(ctx, "/sys/fs/cgroup", relative, id, maximum)
}

func observeGuestMemoryDomainAt(ctx context.Context, directory, relative, id string, maximum int64) (result error) {
	scope, err := guestCgroupScope(relative, id)
	if err != nil || maximum <= 0 || !filepath.IsAbs(directory) || filepath.Clean(directory) != directory {
		return ErrPolicy
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	fd, err := openAbsoluteDirectoryNoLinks(directory)
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
	var rootStat unix.Stat_t
	if unix.Fstat(fd, &rootStat) != nil || rootStat.Uid != 0 || rootStat.Mode&0022 != 0 {
		return ErrPolicy
	}
	components := strings.Split(strings.TrimPrefix(relative, "/"), "/")
	snapshots := make([]unix.Stat_t, 0, len(components))
	for _, component := range components {
		if err := ctx.Err(); err != nil {
			return err
		}
		child, err := openSameMountReadOnly(descriptors[len(descriptors)-1], component, true)
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
		fd, err := openSameMountReadOnly(directory, name, false)
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
	threaded := false
	for i := len(components) - 1; i >= 1; i-- {
		kind, err := read(descriptors[i+1], "cgroup.type")
		if err != nil {
			return err
		}
		types[i] = kind
		if kind == "threaded" {
			threaded = true
			continue
		}
		if (kind != "domain" && kind != "domain threaded") || (threaded && kind != "domain threaded") {
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
	if limit == "" {
		return ErrPolicy
	}
	for _, digit := range limit {
		if digit < '0' || digit > '9' {
			return ErrPolicy
		}
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
		if !samePathMount(descriptors[i+1], descriptors[i], component) {
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
	var currentRoot unix.Stat_t
	if unix.Lstat(directory, &currentRoot) != nil || currentRoot.Dev != rootStat.Dev || currentRoot.Ino != rootStat.Ino || currentRoot.Mode != rootStat.Mode || currentRoot.Uid != rootStat.Uid || currentRoot.Gid != rootStat.Gid {
		return ErrPolicy
	}
	if !samePathMount(fd, unix.AT_FDCWD, directory) {
		return ErrPolicy
	}
	return ctx.Err()
}

// samePathMount requires the current no-follow path and retained descriptor
// to name the same mount; inode equality alone misses a self-bind replacement.
func samePathMount(fd, parent int, name string) bool {
	var retained, current unix.Statx_t
	return unix.Statx(fd, "", unix.AT_EMPTY_PATH|unix.AT_STATX_DONT_SYNC, unix.STATX_MNT_ID, &retained) == nil &&
		unix.Statx(parent, name, unix.AT_SYMLINK_NOFOLLOW|unix.AT_STATX_DONT_SYNC, unix.STATX_MNT_ID, &current) == nil &&
		retained.Mask&unix.STATX_MNT_ID != 0 && current.Mask&unix.STATX_MNT_ID != 0 &&
		retained.Mnt_id != 0 && retained.Mnt_id == current.Mnt_id
}
