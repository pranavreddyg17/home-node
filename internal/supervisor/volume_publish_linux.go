//go:build linux

package supervisor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

// publishReservedVolume publishes only the journaled disk inode, with
// retained namespace and stopped-runtime guards. A completed rename is retried
// through the final path without adopting or replacing another disk.
func (m *Manager) publishReservedVolume(ctx context.Context, parentPath string, d Domain, stopped func(context.Context) error) (VolumeOwnershipIntent, error) {
	if err := ctx.Err(); err != nil {
		return VolumeOwnershipIntent{}, err
	}
	if stopped == nil || !filepath.IsAbs(parentPath) || filepath.Clean(parentPath) != parentPath {
		return VolumeOwnershipIntent{}, ErrPolicy
	}
	saved, err := m.loadVolumeOwnershipIntent(ctx, d)
	if err != nil {
		return VolumeOwnershipIntent{}, err
	}
	stage := filepath.Join(parentPath, "."+d.ID+".volume-prepare")
	final := filepath.Join(parentPath, d.ID+".raw")
	_, stageErr := os.Lstat(stage)
	_, finalErr := os.Lstat(final)
	publish := true
	if stageErr == nil && errors.Is(finalErr, os.ErrNotExist) {
		// Actual source admission occurs under retained descriptors below.
	} else if finalErr == nil && errors.Is(stageErr, os.ErrNotExist) {
		publish = false
	} else {
		return VolumeOwnershipIntent{}, errors.Join(ErrPolicy, stageErr, finalErr)
	}
	err = m.withReservedVolumeOwnership(ctx, parentPath, d, stopped, func(ctx context.Context, directory *os.File, intent VolumeOwnershipIntent, guard func(context.Context) error) error {
		if intent != saved {
			return ErrPolicy
		}
		return guard(ctx)
	}, nil, publish)
	if err != nil {
		return VolumeOwnershipIntent{}, err
	}
	return saved, nil
}
