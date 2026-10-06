package backup

import (
	"context"
	"errors"
	"os"

	"github.com/pranavreddyg17/home-node/internal/state"
)

// quarantineRecoveryManagementStage preserves uncertain private stage bytes
// before an explicitly selected repair. Exclusive root ownership is required.
// Completed originals/rebound outputs, receipts, publications and SQLite sidecars
// are refused. Quarantine grants no authority and never deletes the preserved data.
func quarantineRecoveryManagementStage(ctx context.Context, root *os.Root, manifest Manifest) (name string, result error) {
	plan, err := loadRecoveryInstallPlan(ctx, root)
	if err != nil {
		return "", err
	}
	digest, err := recoveryManifestDigest(manifest)
	if err != nil || digest != plan.ManifestSHA256 {
		return "", ErrManifest
	}
	const stage = ".recovery-management.stage"
	for _, occupied := range []string{"recovery-management.json", "management.db", stage + "-journal", stage + "-wal", stage + "-shm"} {
		if _, err := root.Lstat(occupied); !errors.Is(err, os.ErrNotExist) {
			return "", ErrManifest
		}
	}
	var entry BackupFile
	for _, candidate := range manifest.Files {
		if candidate.Workload == "management" {
			entry = candidate
		}
	}
	if entry.Name != "snapshot.db" || entry.Bytes <= 0 || entry.Bytes > 256<<20 {
		return "", ErrManifest
	}
	entry.Name = stage
	if err = verifyBackupFile(ctx, root, entry); err == nil {
		return "", ErrManifest
	}
	if contextErr := ctx.Err(); contextErr != nil {
		return "", contextErr
	}
	if !errors.Is(err, ErrManifest) {
		return "", err
	}
	if _, err = inspectReboundRecoveryManagement(ctx, root); err == nil {
		return "", ErrManifest
	}
	if contextErr := ctx.Err(); contextErr != nil {
		return "", contextErr
	}
	if !errors.Is(err, ErrManifest) && !errors.Is(err, state.ErrRecovery) {
		return "", err
	}
	expected, err := root.Lstat(stage)
	if err != nil || !expected.Mode().IsRegular() || expected.Mode().Perm() != 0600 || expected.Size() > 256<<20 {
		return "", ErrManifest
	}
	quarantine := ".recovery-management-" + plan.OwnerID + ".quarantine"
	if existing, statErr := root.Lstat(quarantine); statErr == nil {
		if !existing.Mode().IsRegular() || !os.SameFile(expected, existing) {
			return "", ErrManifest
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return "", statErr
	} else if err = root.Link(stage, quarantine); err != nil {
		return "", err
	}
	directory, err := root.Open(".")
	if err != nil {
		return "", err
	}
	defer func() {
		result = errors.Join(result, directory.Close())
		if result != nil {
			name = ""
		}
	}()
	if err = directory.Sync(); err != nil {
		return "", err
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	current, err := root.Lstat(stage)
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(expected, current) {
		return "", ErrManifest
	}
	if err = root.Remove(stage); err != nil {
		return "", err
	}
	if err = directory.Sync(); err != nil {
		return "", err
	}
	return quarantine, ctx.Err()
}
