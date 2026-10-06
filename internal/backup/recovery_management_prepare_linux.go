//go:build linux

package backup

import (
	"context"
	"errors"
	"os"
)

// prepareRecoveryManagement joins disconnected disk and management preparation.
// Both roots must remain exclusively owned. Completed copies and committed
// identity rebinding can resume, but uncertain partial SQLite/copy state is
// refused for explicit repair. This does not install or activate the controller.
func prepareRecoveryManagement(ctx context.Context, source, destination *os.Root, snapshotID string, manifest Manifest, policy RestorePolicy) (string, error) {
	if _, err := prepareRecoveryDisks(ctx, source, destination, snapshotID, manifest, policy); err != nil {
		return "", err
	}
	_, receiptErr := destination.Lstat("recovery-management.json")
	if errors.Is(receiptErr, os.ErrNotExist) {
		// Publication without its receipt cannot establish owned output.
		if _, err := destination.Lstat("management.db"); !errors.Is(err, os.ErrNotExist) {
			return "", ErrManifest
		}
		const stage = ".recovery-management.stage"
		if _, err := destination.Lstat(stage); errors.Is(err, os.ErrNotExist) {
			if err = copyRecoveryManagement(ctx, source, destination, manifest, policy); err != nil {
				return "", err
			}
		} else if err != nil {
			return "", err
		}
		var entry BackupFile
		for _, candidate := range manifest.Files {
			if candidate.Workload == "management" {
				entry = candidate
			}
		}
		entry.Name = stage
		if err := verifyBackupFile(ctx, destination, entry); err == nil {
			if err = rebindRecoveryManagement(ctx, destination, manifest); err != nil {
				return "", err
			}
		} else if !errors.Is(err, ErrManifest) {
			return "", err
		} else if _, err = inspectReboundRecoveryManagement(ctx, destination); err != nil {
			// A completed transaction must have exactly the recorded new identities.
			// Dirty/hot-journal or contradictory metadata is not silently adopted.
			return "", err
		}
		if err := recordRecoveryManagementOutput(ctx, destination); err != nil {
			return "", err
		}
	} else if receiptErr != nil {
		return "", receiptErr
	}
	return publishRecoveryManagement(ctx, destination)
}
