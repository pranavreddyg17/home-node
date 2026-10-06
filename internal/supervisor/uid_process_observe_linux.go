//go:build linux

package supervisor

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
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
	count, processes := 0, 0
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
			processes++
			if processes > 65536 {
				return GuestUIDPool{}, ErrPolicy
			}
			if err = observeGuestUIDTaskConflicts(ctx, root, name, observed, &count); err != nil {
				return GuestUIDPool{}, err
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

// Linux tasks can hold distinct credentials. Enumerating leaders alone would
// miss such authority. A retained process with an unreadable task directory is
// ambiguous and must be refused, including leader-exit races.
func observeGuestUIDTaskConflicts(ctx context.Context, root *os.Root, pid string, pool GuestUIDPool, count *int) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	process, err := root.OpenRoot(pid)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, process.Close()) }()
	tasks, err := process.OpenFile("task", os.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, tasks.Close()) }()
	for {
		if err = ctx.Err(); err != nil {
			return err
		}
		entries, readErr := tasks.ReadDir(256)
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return readErr
		}
		for _, entry := range entries {
			name := entry.Name()
			if name == "" || strings.Trim(name, "0123456789") != "" {
				continue
			}
			*count++
			if *count > 65536 {
				return ErrPolicy
			}
			status, exists, err := readGuestUIDProcessStatus(ctx, process, "task/"+name)
			if err != nil {
				return err
			}
			if !exists {
				continue
			}
			conflicts, err := processGuestUIDConflicts(ctx, GuestUIDPool{First: pool.First, Last: pool.Last}, status)
			if err != nil {
				return err
			}
			for uid := range conflicts.Blocked {
				pool.Blocked[uid] = true
			}
		}
		if errors.Is(readErr, io.EOF) {
			return ctx.Err()
		}
	}
}
