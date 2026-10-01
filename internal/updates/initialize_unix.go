//go:build linux || darwin

package updates

import (
	"context"
	"errors"
	"io"
	"os"

	"github.com/theupdateframework/go-tuf/v2/metadata/trustedmetadata"
	"golang.org/x/sys/unix"
)

// InitializeCacheRoot initializes only an empty, previously provisioned cache.
// It resumes matching initialization intent but never resets an existing cache
// or replaces a rotated root with the immutable bootstrap. Caller must obtain
// bootstrap identity from independently trusted installer configuration.
func InitializeCacheRoot(ctx context.Context, provisioned *os.Root, bootstrap BootstrapRoot) error {
	return initializeCacheRoot(ctx, provisioned, bootstrap, nil)
}

func initializeCacheRoot(ctx context.Context, provisioned *os.Root, bootstrap BootstrapRoot, checkpoint func(string) error) (resultErr error) {
	version, err := bootstrap.Validate()
	if err != nil {
		return err
	}
	cache, err := lockMetadataCache(ctx, provisioned)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, cache.Close()) }()
	mark := []byte(bootstrap.SHA256 + "\n")
	completed, completeErr := readInitializationFile(provisioned, "bootstrap", 65)
	pending, pendingErr := readInitializationFile(provisioned, "bootstrap.pending", 65)
	if completeErr == nil {
		if pendingErr == nil || !os.IsNotExist(pendingErr) || string(completed) != string(mark) {
			return errMetadataCache
		}
		// Completed authority belongs to the current protected root. Signature and
		// version checks detect corruption; trust in its rotation history comes
		// from the exclusively owned cache, not self-signatures alone.
		if err = syncMetadataCacheState(ctx, cache.root, false); err != nil {
			return err
		}
		data, err := readInitializationFile(cache.root, "root.json", 512<<10)
		if err != nil {
			return err
		}
		trusted, err := trustedmetadata.New(data)
		if err != nil || trusted.Root.Signed.Version < version {
			return errMetadataCache
		}
		return nil
	}
	if !os.IsNotExist(completeErr) {
		return completeErr
	}
	if pendingErr != nil && !os.IsNotExist(pendingErr) {
		return pendingErr
	}
	if pendingErr == nil && string(pending) != string(mark) {
		return errMetadataCache
	}
	if _, err = provisioned.Lstat("clock"); !os.IsNotExist(err) {
		return errMetadataCache
	}
	if _, err = provisioned.Lstat("clock.pending"); !os.IsNotExist(err) {
		return errMetadataCache
	}
	directory, err := cache.root.Open(".")
	if err != nil {
		return err
	}
	entries, readErr := directory.ReadDir(2)
	closeErr := directory.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return errors.Join(readErr, closeErr)
	}
	if closeErr != nil {
		return closeErr
	}
	if len(entries) > 1 || len(entries) == 1 && entries[0].Name() != "root.json" {
		return errMetadataCache
	}
	if pendingErr != nil {
		if len(entries) != 0 {
			return errMetadataCache
		}
		if err = writeInitializationFile(provisioned, "bootstrap.pending", mark); err != nil {
			return err
		}
		if checkpoint != nil {
			if err = checkpoint("intent"); err != nil {
				return err
			}
		}
	}
	if len(entries) == 0 {
		if err = writeInitializationFile(cache.root, "root.json", bootstrap.Data); err != nil {
			return err
		}
	} else {
		data, err := readInitializationFile(cache.root, "root.json", 128<<10)
		if err != nil || string(data) != string(bootstrap.Data) {
			return errors.Join(errMetadataCache, err)
		}
	}
	if checkpoint != nil {
		if err = checkpoint("root"); err != nil {
			return err
		}
	}
	if err = syncMetadataCacheState(ctx, cache.root, false); err != nil {
		return err
	}
	if err = provisioned.Rename("bootstrap.pending", "bootstrap"); err != nil {
		return err
	}
	return errors.Join(syncInitializationDirectory(provisioned), ctx.Err())
}

func readInitializationFile(root *os.Root, name string, maximum int64) ([]byte, error) {
	before, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, errMetadataCache
	}
	file, err := root.OpenFile(name, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	var native unix.Stat_t
	if err != nil || !os.SameFile(before, info) || !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 || info.Size() < 1 || info.Size() > maximum || unix.Fstat(int(file.Fd()), &native) != nil || native.Uid != uint32(os.Geteuid()) || native.Nlink != 1 {
		return nil, errMetadataCache
	}
	data, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maximum {
		return nil, errMetadataCache
	}
	return data, nil
}

func writeInitializationFile(root *os.Root, name string, data []byte) error {
	file, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	err = errors.Join(writeErr, file.Sync(), file.Close())
	if err != nil {
		return err
	}
	return syncInitializationDirectory(root)
}

func syncInitializationDirectory(root *os.Root) error {
	directory, err := root.Open(".")
	if err != nil {
		return err
	}
	return errors.Join(directory.Sync(), directory.Close())
}
