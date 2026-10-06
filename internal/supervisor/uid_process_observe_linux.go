//go:build linux

package supervisor

import (
	"context"
	"errors"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"strings"
	"syscall"
	"time"
)

// ObserveGuestUIDProcessConflicts is a bounded current procfs observation, not
// retained exclusion of process creation or credential changes.
func ObserveGuestUIDProcessConflicts(ctx context.Context, pool GuestUIDPool) (observed GuestUIDPool, result error) {
	if err := ctx.Err(); err != nil {
		return observed, err
	}
	if os.Geteuid() != 0 || pool.validate() != nil {
		return observed, ErrPolicy
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	root, err := os.OpenRoot("/proc")
	if err != nil {
		return observed, err
	}
	defer func() { result = errors.Join(result, root.Close()) }()
	directory, err := root.Open(".")
	if err != nil {
		return observed, err
	}
	defer func() { result = errors.Join(result, directory.Close()) }()
	var fs unix.Statfs_t
	if unix.Fstatfs(int(directory.Fd()), &fs) != nil || fs.Type != unix.PROC_SUPER_MAGIC {
		return observed, ErrPolicy
	}
	// A namespaced UID view cannot establish host credential conflicts.
	mappingFile, err := root.OpenFile("self/uid_map", os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return observed, err
	}
	mapping, readMappingErr := io.ReadAll(io.LimitReader(mappingFile, (64<<10)+1))
	err = errors.Join(readMappingErr, mappingFile.Close())
	if err != nil || len(mapping) > 64<<10 || strings.Join(strings.Fields(string(mapping)), " ") != "0 0 4294967295" {
		return observed, ErrPolicy
	}
	observed = GuestUIDPool{First: pool.First, Last: pool.Last, Blocked: make(map[uint32]bool, len(pool.Blocked))}
	for uid, value := range pool.Blocked {
		if value {
			observed.Blocked[uid] = true
		}
	}
	count := 0
	for {
		if err = ctx.Err(); err != nil {
			return GuestUIDPool{}, err
		}
		entries, readErr := directory.ReadDir(256)
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return GuestUIDPool{}, readErr
		}
		for _, entry := range entries {
			name := entry.Name()
			if name == "" || strings.Trim(name, "0123456789") != "" {
				continue
			}
			count++
			if count > 65536 {
				return GuestUIDPool{}, ErrPolicy
			}
			status, exists, err := readGuestUIDProcessStatus(ctx, root, name)
			if err != nil {
				return GuestUIDPool{}, err
			}
			if !exists {
				continue
			}
			conflicts, err := processGuestUIDConflicts(ctx, GuestUIDPool{First: pool.First, Last: pool.Last}, status)
			if err != nil {
				return GuestUIDPool{}, err
			}
			for uid := range conflicts.Blocked {
				observed.Blocked[uid] = true
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
	}
	return observed, ctx.Err()
}

func readGuestUIDProcessStatus(ctx context.Context, root *os.Root, pid string) (data []byte, exists bool, result error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	process, err := root.OpenRoot(pid)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	defer func() { result = errors.Join(result, process.Close()) }()
	file, err := process.OpenFile("status", os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	defer func() { result = errors.Join(result, file.Close()) }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, false, ErrPolicy
	}
	data, err = io.ReadAll(io.LimitReader(file, (64<<10)+1))
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ESRCH) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if len(data) == 0 || len(data) > 64<<10 {
		return nil, false, ErrPolicy
	}
	if err = ctx.Err(); err != nil {
		return nil, false, err
	}
	return data, true, nil
}
