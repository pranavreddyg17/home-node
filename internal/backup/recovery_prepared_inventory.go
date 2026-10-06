package backup

import (
	"context"
	"errors"
	"os"
	"time"
)

// PreparedRecoveryInventory identifies verified disconnected files only. It
// grants no UID lease, runtime policy, client trust or controller activation.
type PreparedRecoveryInventory struct {
	SnapshotID       string
	OwnerID          string
	RecoveryEpoch    int64
	ManagementName   string
	ManagementSHA256 string
	ManagementBytes  int64
	Disks            []RecoveryInstallDisk
}

// InspectPreparedRecovery requires exclusive ownership/lease of private prepared
// storage throughout inspection and later handoff. Manifest is the selected
// restored source; policy is the replacement host's current trusted catalog.
func InspectPreparedRecovery(ctx context.Context, root *os.Root, manifest Manifest, policy RestorePolicy) (PreparedRecoveryInventory, error) {
	plan, err := loadRecoveryInstallPlan(ctx, root)
	if err != nil {
		return PreparedRecoveryInventory{}, err
	}
	digest, err := recoveryManifestDigest(manifest)
	if err != nil || digest != plan.ManifestSHA256 || manifest.Validate(policy, time.Now()) != nil {
		return PreparedRecoveryInventory{}, ErrManifest
	}
	entries := map[string]BackupFile{}
	for _, entry := range manifest.Files {
		if entry.Workload != "management" {
			entries[entry.Workload] = entry
		}
	}
	if len(entries) != len(plan.Disks) {
		return PreparedRecoveryInventory{}, ErrManifest
	}
	for _, disk := range plan.Disks {
		entry, ok := entries[disk.Workload]
		if !ok || entry.Name != disk.SourceName || entry.Bytes != disk.Bytes || entry.SHA256 != disk.SourceSHA256 || entry.ImageSHA256 != disk.ImageSHA256 || policy.ApprovedImages[disk.Workload] != disk.ImageSHA256 {
			return PreparedRecoveryInventory{}, ErrManifest
		}
		stage, err := root.Lstat(".recovery-" + disk.InstanceID + ".stage")
		if err != nil || !stage.Mode().IsRegular() || stage.Mode().Perm() != 0600 {
			return PreparedRecoveryInventory{}, ErrManifest
		}
		final := disk.InstanceID + ".raw"
		before, err := root.Lstat(final)
		if err != nil || !before.Mode().IsRegular() || !os.SameFile(stage, before) || before.Mode().Perm() != 0600 {
			return PreparedRecoveryInventory{}, ErrManifest
		}
		entry.Name = final
		if err = verifyBackupFile(ctx, root, entry); err != nil {
			return PreparedRecoveryInventory{}, err
		}
		file, err := root.Open(final)
		if err != nil {
			return PreparedRecoveryInventory{}, err
		}
		opened, statErr := file.Stat()
		if statErr != nil || !os.SameFile(before, opened) {
			return PreparedRecoveryInventory{}, errors.Join(ErrManifest, file.Close())
		}
		checkErr := QualifyExt4Disk(ctx, file)
		if err = errors.Join(checkErr, file.Close()); err != nil {
			return PreparedRecoveryInventory{}, err
		}
		current, err := root.Lstat(final)
		if err != nil || !current.Mode().IsRegular() || !os.SameFile(opened, current) || current.Size() != disk.Bytes || current.Mode().Perm() != 0600 {
			return PreparedRecoveryInventory{}, ErrManifest
		}
	}
	receipt, err := reconcileRecoveryManagementOutput(ctx, root)
	if err != nil {
		return PreparedRecoveryInventory{}, err
	}
	managementStage, err := root.Lstat(".recovery-management.stage")
	if err != nil {
		return PreparedRecoveryInventory{}, err
	}
	management, err := root.Lstat("management.db")
	if err != nil || !management.Mode().IsRegular() || !os.SameFile(managementStage, management) || management.Mode().Perm() != 0600 || management.Size() != receipt.Output.Bytes {
		return PreparedRecoveryInventory{}, ErrManifest
	}
	if err = ctx.Err(); err != nil {
		return PreparedRecoveryInventory{}, err
	}
	return PreparedRecoveryInventory{SnapshotID: plan.SnapshotID, OwnerID: plan.OwnerID, RecoveryEpoch: plan.RecoveryEpoch, ManagementName: "management.db", ManagementSHA256: receipt.Output.SHA256, ManagementBytes: receipt.Output.Bytes, Disks: append(make([]RecoveryInstallDisk, 0, len(plan.Disks)), plan.Disks...)}, nil
}
