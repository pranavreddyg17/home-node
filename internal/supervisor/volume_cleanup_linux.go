//go:build linux

package supervisor

import (
	"context"
	"encoding/hex"
	"io"
	"os"
	"strings"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"golang.org/x/sys/unix"
)

// Called after the manager admits preparation or stopped-video purge of a recorded resource.
func purgeVolumePreparation(ctx context.Context, directory, id string, size int64) error {
	if os.Geteuid() != 0 || !guestproto.ValidID(id) || size < 16<<20 || size > 512<<30 {
		return ErrPolicy
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0022 != 0 {
		return ErrPolicy
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	parent, err := root.Open(".")
	if err != nil {
		return err
	}
	defer parent.Close()
	opened, err := parent.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return ErrPolicy
	}
	var owner unix.Stat_t
	if unix.Fstat(int(parent.Fd()), &owner) != nil || owner.Uid != 0 {
		return ErrPolicy
	}
	prefix := id + ".raw.prepare-"
	type candidate struct {
		name string
		info os.FileInfo
	}
	var candidates []candidate
	total := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		entries, readErr := parent.ReadDir(128)
		total += len(entries)
		if total > 4096 {
			return ErrPolicy
		}
		for _, entry := range entries {
			if !strings.HasPrefix(entry.Name(), prefix) {
				continue
			}
			suffix := strings.TrimPrefix(entry.Name(), prefix)
			nonce, err := hex.DecodeString(suffix)
			if err != nil || len(nonce) != 16 || hex.EncodeToString(nonce) != suffix || len(candidates) >= 32 {
				return ErrPolicy
			}
			file, err := root.OpenFile(entry.Name(), unix.O_PATH|unix.O_NOFOLLOW, 0)
			if err != nil {
				return err
			}
			observed, err := file.Stat()
			var metadata unix.Stat_t
			valid := err == nil && observed.Mode().IsRegular() && observed.Mode().Perm() == 0600 && observed.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) == 0 && (observed.Size() == 0 || observed.Size() == size)
			valid = valid && unix.Fstat(int(file.Fd()), &metadata) == nil && metadata.Uid == 0 && metadata.Nlink == 1
			file.Close()
			if !valid {
				return ErrPolicy
			}
			candidates = append(candidates, candidate{entry.Name(), observed})
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	// Admit the complete set before unlinking any staged file; never erase final .raw files here.
	for _, item := range candidates {
		if err := ctx.Err(); err != nil {
			return err
		}
		current, err := root.Lstat(item.name)
		if err != nil || !os.SameFile(item.info, current) || current.Mode() != item.info.Mode() || current.Size() != item.info.Size() {
			return ErrPolicy
		}
		if err := root.Remove(item.name); err != nil {
			return err
		}
	}
	return parent.Sync()
}
