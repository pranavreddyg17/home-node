//go:build linux

package supervisor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

// publishReservedChannel publishes only the journaled directory inode, with
// retained namespace and stopped-runtime guards. A completed rename is retried
// through the final path without adopting or replacing another directory.
func (m *Manager) publishReservedChannel(ctx context.Context, parentPath string, d Domain, stopped func(context.Context) error) (ChannelOwnershipIntent, error) {
	if err := ctx.Err(); err != nil {
		return ChannelOwnershipIntent{}, err
	}
	if stopped == nil || !filepath.IsAbs(parentPath) || filepath.Clean(parentPath) != parentPath {
		return ChannelOwnershipIntent{}, ErrPolicy
	}
	saved, err := m.loadChannelOwnershipIntent(ctx, d)
	if err != nil {
		return ChannelOwnershipIntent{}, err
	}
	stage := filepath.Join(parentPath, "."+d.ID+".channel-prepare")
	final := filepath.Join(parentPath, d.ID)
	_, stageErr := os.Lstat(stage)
	_, finalErr := os.Lstat(final)
	path, publish := stage, true
	if stageErr == nil && errors.Is(finalErr, os.ErrNotExist) {
		// Actual source admission occurs under retained descriptors below.
	} else if finalErr == nil && errors.Is(stageErr, os.ErrNotExist) {
		path, publish = final, false
	} else {
		return ChannelOwnershipIntent{}, errors.Join(ErrPolicy, stageErr, finalErr)
	}
	err = m.withReservedChannelEntry(ctx, path, d, stopped, func(ctx context.Context, directory *os.File, intent ChannelOwnershipIntent, guard func(context.Context) error) error {
		if intent != saved {
			return ErrPolicy
		}
		return guard(ctx)
	}, publish, nil)
	if err != nil {
		return ChannelOwnershipIntent{}, err
	}
	return saved, nil
}
