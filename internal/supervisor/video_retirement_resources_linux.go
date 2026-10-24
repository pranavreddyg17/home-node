//go:build linux

package supervisor

import (
	"context"
	"database/sql"
	"errors"
	"os"
)

// Resume only the physical scopes allowed by the authenticated durable phase.
// The caller retains runtime/start exclusion and supplies qualified process
// absence. All component descriptors close before return; ownership and UID are
// retained even on success. Completion and safe UID reuse are separate gates.
func (m *Manager) retireVideoResources(ctx context.Context, supplied videoRetirementRecord, stopped func(context.Context) error) (record videoRetirementRecord, result error) {
	defer func() {
		if result != nil {
			record = videoRetirementRecord{}
		}
	}()
	if m == nil || m.Store == nil || stopped == nil || os.Geteuid() != 0 {
		return record, ErrPolicy
	}
	if err := stopped(ctx); err != nil {
		return record, err
	}
	record = supplied
	if err := m.Store.Transaction(ctx, func(tx *sql.Tx) error {
		phase, err := m.readVideoRetirementRecordTx(ctx, tx, record.Intent)
		if err != nil {
			return err
		}
		record.Stage = phase
		return ctx.Err()
	}); err != nil {
		return record, err
	}
	for step := 0; step < 4; step++ {
		var err error
		switch record.Stage {
		case "prepared", "socket-removing":
			record, err = m.retireVideoSocketWithVolume(ctx, record, stopped)
		case "socket-removed", "volume-removing":
			record, err = m.retireVideoVolume(ctx, record, stopped)
		case "volume-removed", "directory-removing":
			record, err = m.retireVideoDirectory(ctx, record, stopped)
		case "directory-removed":
			// Requalify final absence even for an already-finished physical replay.
			record, err = m.retireVideoDirectory(ctx, record, stopped)
			if err != nil {
				return record, err
			}
			if err := stopped(ctx); err != nil {
				return record, err
			}
			return record, nil
		default:
			return record, ErrPolicy
		}
		if err != nil {
			return record, err
		}
	}
	return record, errors.Join(ErrPolicy, ctx.Err())
}
