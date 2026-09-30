package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"time"

	"github.com/pranavreddyg17/home-node/internal/state"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

type MaintenanceDisks interface {
	WithMaintenanceDisk(context.Context, string, string, func(context.Context, *os.File, supervisor.Instance) error) error
}

// StageRecoverySet builds a validated recovery inventory using owned management
// maintenance and supervisor disk callbacks. The coordinator must qualify clean
// guest filesystems, freeze other writers and retain exclusive staging ownership.
// This builder does not enroll a drive, encrypt/publish a backup or restart apps.
func StageRecoverySet(ctx context.Context, store *state.Store, disks MaintenanceDisks, managementToken, runtimeToken string, root *os.Root, release string, catalogVersion int64, policy RestorePolicy) (result Manifest, resultErr error) {
	if store == nil || disks == nil || root == nil || managementToken == "" || runtimeToken == "" {
		return result, ErrManifest
	}
	manifest := Manifest{Version: 1, CreatedAt: time.Now().UTC(), Release: release, Platform: "ubuntu-24.04-amd64", ManagementSchema: 4, CatalogVersion: catalogVersion, Files: []BackupFile{{Workload: "management", Name: "snapshot.db", Bytes: 1, SHA256: hex.EncodeToString(make([]byte, 32)), DataSchema: 4}}}
	if err := manifest.Validate(policy, time.Now()); err != nil {
		return result, err
	}
	directory, err := root.Open(".")
	if err != nil {
		return result, err
	}
	defer directory.Close()
	info, err := directory.Stat()
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return result, ErrManifest
	}
	entries, err := directory.ReadDir(1)
	if len(entries) != 0 || err != io.EOF {
		return result, ErrManifest
	}
	created := map[string]os.FileInfo{}
	defer func() {
		if resultErr != nil {
			for name, original := range created {
				current, err := root.Lstat(name)
				if err == nil && os.SameFile(original, current) {
					resultErr = errors.Join(resultErr, root.Remove(name))
				}
			}
			resultErr = errors.Join(resultErr, directory.Sync())
			result = Manifest{}
		}
	}()
	if _, err = store.MaintenanceRecoverySnapshot(ctx, managementToken, root.Name()); err != nil {
		return result, err
	}
	snapshot, err := root.Open("snapshot.db")
	if err != nil {
		return result, err
	}
	defer snapshot.Close()
	info, err = snapshot.Stat()
	if err != nil {
		return result, err
	}
	created["snapshot.db"] = info
	apps, err := state.ValidateRecoverySnapshot(ctx, snapshot)
	if err != nil {
		return result, err
	}
	hash := sha256.New()
	reader := io.NewSectionReader(snapshot, 0, info.Size())
	buffer := make([]byte, 1<<20)
	remaining := info.Size()
	for remaining > 0 {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		n, err := io.ReadFull(reader, buffer[:min(remaining, int64(len(buffer)))])
		if err != nil {
			return result, err
		}
		_, _ = hash.Write(buffer[:n])
		remaining -= int64(n)
	}
	manifest.Files[0].Bytes = info.Size()
	manifest.Files[0].SHA256 = hex.EncodeToString(hash.Sum(nil))
	for _, app := range apps {
		err = disks.WithMaintenanceDisk(ctx, runtimeToken, app.InstanceID, func(ctx context.Context, file *os.File, instance supervisor.Instance) error {
			if instance.ID != app.InstanceID || instance.Workload != app.Workload || instance.State != "stopped" || instance.Desired != "stopped" || policy.ApprovedImages[app.Workload] != instance.ImageSHA256 {
				return ErrManifest
			}
			entry, err := StageDisk(ctx, root, file, BackupFile{Workload: app.Workload, Name: app.Workload + ".raw", Bytes: instance.DataBytes, ImageSHA256: instance.ImageSHA256, DataSchema: 1, Protocol: 1})
			if err != nil {
				return err
			}
			info, err := root.Lstat(entry.Name)
			if err != nil {
				return err
			}
			created[entry.Name] = info
			manifest.Files = append(manifest.Files, entry)
			return nil
		})
		if err != nil {
			return result, err
		}
	}
	inventory, err := store.InspectMaintenance(ctx, managementToken)
	if err != nil {
		return result, err
	}
	if inventory != (state.MaintenanceInventory{}) {
		return result, state.ErrMaintenance
	}
	if err = ValidateRecoverySet(ctx, root, manifest, policy); err != nil {
		return result, err
	}
	return manifest, nil
}
