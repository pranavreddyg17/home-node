package backup

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/pranavreddyg17/home-node/internal/state"
)

// ValidateRecoverySet adds database authority/inventory checks to payload
// verification. It does not install disks, enroll clients or start workloads.
func ValidateRecoverySet(ctx context.Context, root *os.Root, manifest Manifest, policy RestorePolicy) error {
	if root == nil {
		return ErrManifest
	}
	info, err := root.Stat(".")
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return ErrManifest
	}
	if err := VerifyPayload(ctx, root, manifest, policy); err != nil {
		return err
	}
	snapshot, err := root.Open("snapshot.db")
	if err != nil {
		return ErrManifest
	}
	defer snapshot.Close()
	apps, err := state.ValidateRecoverySnapshot(ctx, snapshot)
	if err != nil {
		return ErrManifest
	}
	declared := map[string]bool{}
	for _, file := range manifest.Files {
		if file.Workload != "management" {
			declared[file.Workload] = true
		}
	}
	for _, app := range apps {
		if !declared[app.Workload] {
			return ErrManifest
		}
		delete(declared, app.Workload)
	}
	if len(declared) != 0 || manifest.Validate(policy, time.Now()) != nil {
		return ErrManifest
	}
	return nil
}

// QualifyRecoveryDisks checks restored filesystems after payload and authority
// validation. The caller must retain exclusive ownership of staging throughout
// checking and installation. This does not reconstruct policy, enroll devices,
// establish application consistency, or authorize runtime activation.
func QualifyRecoveryDisks(ctx context.Context, root *os.Root, manifest Manifest, policy RestorePolicy) error {
	if err := ValidateRecoverySet(ctx, root, manifest, policy); err != nil {
		return err
	}
	for _, entry := range manifest.Files {
		if entry.Workload == "management" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		expected, err := root.Lstat(entry.Name)
		if err != nil || !expected.Mode().IsRegular() {
			return ErrManifest
		}
		disk, err := root.Open(entry.Name)
		if err != nil {
			return ErrManifest
		}
		info, statErr := disk.Stat()
		if statErr != nil || !os.SameFile(expected, info) {
			return errors.Join(ErrManifest, disk.Close())
		}
		checkErr := QualifyExt4Disk(ctx, disk)
		current, pathErr := root.Lstat(entry.Name)
		if pathErr != nil || !current.Mode().IsRegular() || !os.SameFile(info, current) {
			checkErr = errors.Join(checkErr, ErrManifest)
		}
		if err := errors.Join(checkErr, disk.Close()); err != nil {
			return err
		}
	}
	return ctx.Err()
}
