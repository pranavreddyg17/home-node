package backup

import (
	"context"
	"errors"
	"os"

	"github.com/pranavreddyg17/home-node/internal/state"
)

// copyRecoveryManagement stages the original sanitized management database under
// journaled intent. Both roots must remain exclusively owned and disconnected.
// It does not rebind identity, publish a live database or enroll any clients.
func copyRecoveryManagement(ctx context.Context, source, destination *os.Root, manifest Manifest, policy RestorePolicy) error {
	plan, err := loadRecoveryInstallPlan(ctx, destination)
	if err != nil {
		return err
	}
	if err = requalifyRecoveryInstallPlan(ctx, source, plan, manifest, policy); err != nil {
		return err
	}
	var entry BackupFile
	for _, candidate := range manifest.Files {
		if candidate.Workload == "management" {
			entry = candidate
		}
	}
	if entry.Name != "snapshot.db" || entry.Bytes > 256<<20 {
		return ErrManifest
	}
	const target = ".recovery-management.stage"
	if err = copyRecoveryPayload(ctx, source, destination, entry, target); err != nil {
		return err
	}
	file, err := destination.Open(target)
	if err != nil {
		return err
	}
	_, err = state.ValidateRecoverySnapshot(ctx, file)
	return errors.Join(err, file.Close())
}
