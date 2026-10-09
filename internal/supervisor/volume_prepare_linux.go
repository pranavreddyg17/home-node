//go:build linux

package supervisor

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
)

// prepareReservedVolume resumes only immutable, inode-qualified preparation.
// The caller retains runtime exclusion throughout; this never reformats recorded data and
// grants no authority to launch a guest. Unrecorded partial effects are refused.
func (m *Manager) prepareReservedVolume(ctx context.Context, parentPath string, d Domain, reserve int64, stopped func(context.Context) error) (VolumeOwnershipIntent, error) {
	if stopped == nil || !filepath.IsAbs(parentPath) || filepath.Clean(parentPath) != parentPath {
		return VolumeOwnershipIntent{}, ErrPolicy
	}
	if err := m.checkVolumePreparationDomain(ctx, d); err != nil {
		return VolumeOwnershipIntent{}, err
	}
	saved, err := m.loadVolumeOwnershipIntent(ctx, d)
	if errors.Is(err, sql.ErrNoRows) {
		saved, err = m.stageReservedVolume(ctx, parentPath, d, reserve, stopped)
	}
	if err != nil {
		return VolumeOwnershipIntent{}, err
	}
	published, err := m.publishReservedVolume(ctx, parentPath, d, stopped)
	if err != nil || published != saved {
		return VolumeOwnershipIntent{}, errors.Join(ErrPolicy, err)
	}
	transferred, err := m.transferReservedVolume(ctx, parentPath, d, stopped)
	if err != nil || transferred != saved {
		return VolumeOwnershipIntent{}, errors.Join(ErrPolicy, err)
	}
	return transferred, nil
}
