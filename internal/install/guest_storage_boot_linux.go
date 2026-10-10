//go:build linux

package install

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

func guestStorageBootIDAdmitted(value string) bool {
	if len(value) != 36 || value == "00000000-0000-0000-0000-000000000000" {
		return false
	}
	for i, c := range value {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !strings.ContainsRune("0123456789abcdef", c) {
			return false
		}
	}
	return true
}

// Read the fixed kernel boot identity from procfs, not a configurable file.
// Runtime inode numbers can be reused after reboot; a receipt must additionally
// belong to the current boot before it can authorize any directory publication.
func observeGuestStorageBootID(ctx context.Context) (boot string, result error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	file, err := os.OpenFile("/proc/sys/kernel/random/boot_id", os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return "", err
	}
	defer func() {
		result = errors.Join(result, file.Close())
		if result != nil {
			boot = ""
		}
	}()
	var filesystem unix.Statfs_t
	var metadata unix.Stat_t
	if unix.Fstatfs(int(file.Fd()), &filesystem) != nil || filesystem.Type != unix.PROC_SUPER_MAGIC || unix.Fstat(int(file.Fd()), &metadata) != nil || metadata.Mode != unix.S_IFREG|0444 || metadata.Uid != 0 || metadata.Gid != 0 {
		return "", ErrConflict
	}
	data, err := io.ReadAll(io.LimitReader(file, 38))
	if err != nil {
		return "", err
	}
	if len(data) != 37 || data[36] != '\n' || !guestStorageBootIDAdmitted(string(data[:36])) {
		return "", ErrConflict
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return string(data[:36]), nil
}
