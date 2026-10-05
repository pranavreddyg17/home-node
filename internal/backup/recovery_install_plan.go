package backup

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
)

type recoveryInstallPlan struct {
	Version    int                   `json:"version"`
	SnapshotID string                `json:"snapshotId"`
	Disks      []RecoveryInstallDisk `json:"disks"`
}

func (p recoveryInstallPlan) validate() error {
	if p.Version != 1 || !repositoryPattern.MatchString(p.SnapshotID) || p.Disks == nil || len(p.Disks) > 2 {
		return ErrManifest
	}
	workloads, identities := map[string]bool{}, map[string]bool{}
	for _, disk := range p.Disks {
		if (disk.Workload != "files" && disk.Workload != "ai") || workloads[disk.Workload] || identities[disk.InstanceID] || disk.SourceName != disk.Workload+".raw" || disk.Bytes <= 0 || disk.Bytes > 512<<30 || !repositoryPattern.MatchString(disk.SourceSHA256) || !repositoryPattern.MatchString(disk.ImageSHA256) || !guestproto.ValidID(disk.InstanceID) {
			return ErrManifest
		}
		workloads[disk.Workload], identities[disk.InstanceID] = true, true
	}
	return nil
}

// createRecoveryInstallPlan persists immutable target identities before effects.
// A caller must own the private journal root exclusively and supply inventory
// from qualification. This journal alone grants no filesystem/runtime authority.
// Existing plans are never overwritten or adopted by this creation operation.
func createRecoveryInstallPlan(ctx context.Context, root *os.Root, plan recoveryInstallPlan) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if root == nil || plan.validate() != nil {
		return ErrManifest
	}
	info, err := root.Stat(".")
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return ErrManifest
	}
	data, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	directory, err := root.Open(".")
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, directory.Close()) }()
	file, err := root.OpenFile("recovery-install.json", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	created, err := file.Stat()
	if err != nil {
		return errors.Join(err, file.Close())
	}
	defer func() {
		result = errors.Join(result, file.Close())
		if result != nil {
			result = errors.Join(result, removeOwnedStaging(root, map[string]os.FileInfo{"recovery-install.json": created}), directory.Sync())
		}
	}()
	if err = ctx.Err(); err != nil {
		return err
	}
	if n, writeErr := file.Write(data); writeErr != nil || n != len(data) {
		return errors.Join(ErrManifest, writeErr)
	}
	if err = file.Sync(); err != nil {
		return err
	}
	return errors.Join(directory.Sync(), ctx.Err())
}
