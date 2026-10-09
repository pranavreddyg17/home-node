//go:build linux

package supervisor

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// This qualifies a lookup only. The connecting caller must still authenticate
// its live peer; a returned pathname does not retain connection authority.
func (m *Manager) qualifyReservedChannelPath(ctx context.Context, d Domain, revision int64) (result error) {
	intent, err := m.loadActiveChannelSocketIntent(ctx, d, revision)
	if err != nil {
		return err
	}
	path := filepath.Join(m.Channels, d.ID, "adapter.sock")
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, root.Close()) }()
	directory, err := root.Open(".")
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, directory.Close()) }()
	socket, err := root.OpenFile("adapter.sock", unix.O_PATH|unix.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, socket.Close()) }()
	checkPath := func() error {
		opened, err := directory.Stat()
		named, pathErr := os.Lstat(filepath.Dir(path))
		if err != nil || pathErr != nil || !os.SameFile(opened, named) || !samePathMount(int(directory.Fd()), unix.AT_FDCWD, filepath.Dir(path)) {
			return errors.Join(ErrPolicy, err, pathErr)
		}
		return ctx.Err()
	}
	if err := checkPath(); err != nil {
		return err
	}
	actual, err := m.checkPinnedChannelSocket(ctx, revision, intent.Channel, directory, socket, false)
	if err != nil || actual != intent {
		return errors.Join(ErrPolicy, err)
	}
	return checkPath()
}
