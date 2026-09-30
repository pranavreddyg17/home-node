package state

import (
	"context"
	"errors"
	"os"
)

// MaintenanceRecoverySnapshot requires owned, drained management inventory
// before and after SQLite backup. The coordinator must separately hold the
// supervisor disk lease and freeze other configuration/byte writers. This is
// not by itself a complete consistent app-disk backup set.
func (s *Store) MaintenanceRecoverySnapshot(ctx context.Context, token, directory string) (string, error) {
	check := func() error {
		inventory, err := s.InspectMaintenance(ctx, token)
		if err != nil {
			return err
		}
		if inventory != (MaintenanceInventory{}) {
			return ErrMaintenance
		}
		return nil
	}
	if err := check(); err != nil {
		return "", err
	}
	path, err := s.RecoverySnapshot(ctx, directory)
	if err != nil {
		return "", err
	}
	if err := check(); err != nil {
		// RecoverySnapshot exclusively created this file in private staging.
		// The coordinator owns the staging root throughout this operation.
		removeErr := os.Remove(path)
		dir, openErr := os.Open(directory)
		if openErr != nil {
			return "", errors.Join(err, removeErr, openErr)
		}
		return "", errors.Join(err, removeErr, dir.Sync(), dir.Close())
	}
	return path, nil
}
