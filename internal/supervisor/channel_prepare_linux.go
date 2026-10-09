//go:build linux

package supervisor

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
)

// prepareReservedChannel resumes only immutable, inode-qualified preparation.
// The caller retains runtime exclusion throughout; this creates no socket and
// grants no authority to launch a guest. Unrecorded partial effects are refused.
func (m *Manager) prepareReservedChannel(ctx context.Context, parentPath string, d Domain, stopped func(context.Context) error) (ChannelOwnershipIntent, error) {
	if stopped == nil || !filepath.IsAbs(parentPath) || filepath.Clean(parentPath) != parentPath {
		return ChannelOwnershipIntent{}, ErrPolicy
	}
	if err := m.checkChannelPreparationDomain(ctx, d); err != nil {
		return ChannelOwnershipIntent{}, err
	}
	saved, err := m.loadChannelOwnershipIntent(ctx, d)
	if errors.Is(err, sql.ErrNoRows) {
		saved, err = m.stageReservedChannel(ctx, parentPath, d, stopped)
	}
	if err != nil {
		return ChannelOwnershipIntent{}, err
	}
	published, err := m.publishReservedChannel(ctx, parentPath, d, stopped)
	if err != nil || published != saved {
		return ChannelOwnershipIntent{}, errors.Join(ErrPolicy, err)
	}
	transferred, err := m.transferReservedChannel(ctx, filepath.Join(parentPath, d.ID), d, stopped)
	if err != nil || transferred != saved {
		return ChannelOwnershipIntent{}, errors.Join(ErrPolicy, err)
	}
	return transferred, nil
}
