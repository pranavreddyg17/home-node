//go:build linux

package backup

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strconv"
	"time"

	"github.com/pranavreddyg17/home-node/internal/state"
)

// rebindRecoveryManagement changes only an original verified private management
// stage. Caller retains exclusive destination ownership and disconnected state.
// Its source checksum ceases to identify the stage after commit; later crash
// reconciliation/publication must track the resulting database separately.
func rebindRecoveryManagement(ctx context.Context, root *os.Root, manifest Manifest) error {
	deadline, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	plan, err := loadRecoveryInstallPlan(deadline, root)
	if err != nil {
		return err
	}
	digest, err := recoveryManifestDigest(manifest)
	if err != nil || digest != plan.ManifestSHA256 {
		return ErrManifest
	}
	var entry BackupFile
	for _, candidate := range manifest.Files {
		if candidate.Workload == "management" {
			entry = candidate
		}
	}
	const stage = ".recovery-management.stage"
	if entry.Name != "snapshot.db" || entry.Bytes <= 0 || entry.Bytes > 256<<20 {
		return ErrManifest
	}
	entry.Name = stage
	if err = verifyBackupFile(deadline, root, entry); err != nil {
		return err
	}
	file, err := root.Open(stage)
	if err != nil {
		return err
	}
	expected, err := file.Stat()
	if err != nil || !expected.Mode().IsRegular() || expected.Mode().Perm() != 0600 {
		return errors.Join(ErrManifest, file.Close())
	}
	metadata, readErr := state.ReadRecoveryMetadata(deadline, file)
	if err = errors.Join(readErr, file.Close()); err != nil {
		return err
	}
	if metadata.OwnerID == plan.OwnerID || metadata.Epoch >= plan.RecoveryEpoch || plan.RecoveryEpoch-metadata.Epoch != 1 {
		return ErrManifest
	}
	directory, err := root.Open(".")
	if err != nil {
		return err
	}
	defer directory.Close()
	// Address SQLite through the retained directory descriptor. Exclusive root
	// ownership remains required throughout SQLite's path and journal handling.
	uri := "file:/proc/self/fd/" + strconv.FormatUint(uint64(directory.Fd()), 10) + "/" + stage + "?mode=rw&_pragma=trusted_schema(OFF)&_pragma=foreign_keys(1)&_pragma=journal_mode(DELETE)&_pragma=synchronous(FULL)"
	db, err := sql.Open("sqlite", uri)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	defer db.Close()
	tx, err := db.BeginTx(deadline, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	instances := map[string]string{}
	for _, disk := range plan.Disks {
		instances[disk.Workload] = disk.InstanceID
	}
	if err = state.RebindRecoveryTx(deadline, tx, plan.OwnerID, plan.RecoveryEpoch, instances); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	if err = db.Close(); err != nil {
		return err
	}
	current, err := root.Lstat(stage)
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(expected, current) || current.Mode().Perm() != 0600 {
		return ErrManifest
	}
	synced, err := root.OpenFile(stage, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	if err = errors.Join(synced.Sync(), synced.Close(), directory.Sync()); err != nil {
		return err
	}
	return deadline.Err()
}
