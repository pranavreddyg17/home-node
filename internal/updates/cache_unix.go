//go:build linux || darwin

package updates

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

var errMetadataCache = errors.New("update metadata cache is unsafe or incomplete")

// syncMetadataCache flushes a successful TUF refresh before release authority
// is used. The caller must hold the exclusive updater lock and provide the
// pinned, privately owned metadata directory used by the updater. It does not
// validate signatures or select the trusted bootstrap/current root.
func syncMetadataCache(ctx context.Context, root *os.Root) error {
	if root == nil {
		return errMetadataCache
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	directory, err := root.Open(".")
	if err != nil {
		return err
	}
	defer directory.Close()
	info, err := directory.Stat()
	var native unix.Stat_t
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 || unix.Fstat(int(directory.Fd()), &native) != nil || native.Uid != uint32(os.Geteuid()) {
		return errMetadataCache
	}
	entries, err := directory.ReadDir(65)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if len(entries) > 64 {
		return errMetadataCache
	}
	required := map[string]bool{"root.json": false, "timestamp.json": false, "snapshot.json": false, "targets.json": false}
	var total int64
	for _, entry := range entries {
		if err = ctx.Err(); err != nil {
			return err
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") || entry.IsDir() {
			return errMetadataCache
		}
		file, err := root.OpenFile(name, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
		if err != nil {
			return errors.Join(errMetadataCache, err)
		}
		err = func() error {
			defer file.Close()
			info, err := file.Stat()
			var native unix.Stat_t
			if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 || info.Size() < 1 || info.Size() > maxMetadataDownload || unix.Fstat(int(file.Fd()), &native) != nil || native.Uid != uint32(os.Geteuid()) || native.Nlink != 1 {
				return errMetadataCache
			}
			total += info.Size()
			if total > 128<<20 {
				return errMetadataCache
			}
			if err = file.Sync(); err != nil {
				return err
			}
			current, err := root.Lstat(name)
			if err != nil || !os.SameFile(info, current) {
				return errMetadataCache
			}
			return nil
		}()
		if err != nil {
			return err
		}
		if _, ok := required[name]; ok {
			required[name] = true
		}
	}
	for _, present := range required {
		if !present {
			return errMetadataCache
		}
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return directory.Sync()
}
