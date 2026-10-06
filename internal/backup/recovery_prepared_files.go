package backup

import (
	"context"
	"errors"
	"os"
	"time"
)

// PreparedRecoveryFile is a borrowed read-only descriptor. Consumers must not
// close or retain it beyond WithFiles, or alter the private preparation root.
type PreparedRecoveryFile struct {
	Name   string
	Bytes  int64
	SHA256 string
	File   *os.File
}

// WithFiles retains exclusion and root lifetime through verification, handoff
// and descriptor cleanup. The callback must journal its destination intent
// before effects; this operation grants no ownership or runtime authority.
// Callbacks must not invoke Close or another operation on the same lease.
func (l *PreparedRecoveryLease) WithFiles(ctx context.Context, expectedUID uint32, manifest Manifest, policy RestorePolicy, consume func(context.Context, PreparedRecoveryInventory, []PreparedRecoveryFile) error) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if l == nil || l.ownerUID != expectedUID || consume == nil {
		return ErrManifest
	}
	if !l.mu.TryLock() {
		return ErrMaintenanceRunner
	}
	defer l.mu.Unlock()
	if l.closed {
		return ErrManifest
	}
	deadline, cancel := context.WithTimeout(ctx, 2*time.Hour)
	defer cancel()
	inventory, err := InspectPreparedRecovery(deadline, l.root, manifest, policy)
	if err != nil {
		return err
	}
	files := make([]PreparedRecoveryFile, 0, len(inventory.Disks)+1)
	defer func() {
		for _, file := range files {
			result = errors.Join(result, file.File.Close())
		}
	}()
	open := func(name, stage, hash string, size int64) error {
		before, err := l.root.Lstat(name)
		if err != nil || !before.Mode().IsRegular() || before.Mode().Perm() != 0600 || before.Size() != size {
			return ErrManifest
		}
		proof, err := l.root.Lstat(stage)
		if err != nil || !os.SameFile(before, proof) {
			return ErrManifest
		}
		file, err := l.root.Open(name)
		if err != nil {
			return err
		}
		info, err := file.Stat()
		if err != nil || !os.SameFile(before, info) {
			return errors.Join(ErrManifest, file.Close())
		}
		files = append(files, PreparedRecoveryFile{Name: name, Bytes: size, SHA256: hash, File: file})
		return deadline.Err()
	}
	if err = open(inventory.ManagementName, ".recovery-management.stage", inventory.ManagementSHA256, inventory.ManagementBytes); err != nil {
		return err
	}
	for _, disk := range inventory.Disks {
		if err = open(disk.InstanceID+".raw", ".recovery-"+disk.InstanceID+".stage", disk.SourceSHA256, disk.Bytes); err != nil {
			return err
		}
	}
	// Keep cleanup handles private even when a consumer edits its inventory.
	if err = consume(deadline, inventory, append([]PreparedRecoveryFile(nil), files...)); err != nil {
		return err
	}
	return deadline.Err()
}
