package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"
	"unicode/utf8"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
)

type recoveryInstallPlan struct {
	Version    int                   `json:"version"`
	SnapshotID string                `json:"snapshotId"`
	Disks      []RecoveryInstallDisk `json:"disks"`
}

// requalifyRecoveryInstallPlan preserves recorded target identities while
// checking their source mappings against the selected restored manifest and
// current trusted policy. Exclusive staging ownership must continue through
// subsequent copying; this does not authorize runtime activation.
func requalifyRecoveryInstallPlan(ctx context.Context, source *os.Root, plan recoveryInstallPlan, manifest Manifest, policy RestorePolicy) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if plan.validate() != nil || manifest.Validate(policy, time.Now()) != nil {
		return ErrManifest
	}
	entries := map[string]BackupFile{}
	for _, entry := range manifest.Files {
		if entry.Workload != "management" {
			entries[entry.Workload] = entry
		}
	}
	if len(entries) != len(plan.Disks) {
		return ErrManifest
	}
	for _, disk := range plan.Disks {
		entry, ok := entries[disk.Workload]
		if !ok || disk.SourceName != entry.Name || disk.Bytes != entry.Bytes || disk.SourceSHA256 != entry.SHA256 || disk.ImageSHA256 != entry.ImageSHA256 || disk.ImageSHA256 != policy.ApprovedImages[disk.Workload] {
			return ErrManifest
		}
	}
	return QualifyRecoveryDisks(ctx, source, manifest, policy)
}

func decodeRecoveryInstallPlan(data []byte) (recoveryInstallPlan, error) {
	if len(data) == 0 || len(data) > 4096 || !utf8.Valid(data) || uniqueJSON(json.NewDecoder(bytes.NewReader(data)), 0) != nil {
		return recoveryInstallPlan{}, ErrManifest
	}
	fields, err := exactManifestObject(data, []string{"version", "snapshotId", "disks"}, "")
	if err != nil {
		return recoveryInstallPlan{}, err
	}
	var disks []json.RawMessage
	if json.Unmarshal(fields["disks"], &disks) != nil || disks == nil || len(disks) > 2 {
		return recoveryInstallPlan{}, ErrManifest
	}
	for _, disk := range disks {
		if _, err = exactManifestObject(disk, []string{"workload", "sourceName", "bytes", "sourceSha256", "imageSha256", "instanceId"}, ""); err != nil {
			return recoveryInstallPlan{}, err
		}
	}
	var plan recoveryInstallPlan
	if json.Unmarshal(data, &plan) != nil || plan.validate() != nil {
		return recoveryInstallPlan{}, ErrManifest
	}
	return plan, nil
}

// loadRecoveryInstallPlan only reopens the recorded immutable plan. Exclusive
// ownership of the private journal root is required; callers must separately
// requalify source data against current trusted policy before any disk effects.
func loadRecoveryInstallPlan(ctx context.Context, root *os.Root) (plan recoveryInstallPlan, result error) {
	if err := ctx.Err(); err != nil {
		return plan, err
	}
	if root == nil {
		return plan, ErrManifest
	}
	info, err := root.Stat(".")
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return plan, ErrManifest
	}
	expected, err := root.Lstat("recovery-install.json")
	if err != nil {
		return plan, err
	}
	if !expected.Mode().IsRegular() || expected.Mode().Perm() != 0600 || expected.Size() <= 0 || expected.Size() > 4096 {
		return plan, ErrManifest
	}
	file, err := root.Open("recovery-install.json")
	if err != nil {
		return plan, err
	}
	defer func() {
		result = errors.Join(result, file.Close())
		if result != nil {
			plan = recoveryInstallPlan{}
		}
	}()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(expected, opened) {
		return plan, ErrManifest
	}
	data, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil {
		return plan, err
	}
	plan, err = decodeRecoveryInstallPlan(data)
	if err != nil {
		return recoveryInstallPlan{}, err
	}
	current, err := root.Lstat("recovery-install.json")
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(opened, current) || current.Mode().Perm() != 0600 || current.Size() != int64(len(data)) {
		return recoveryInstallPlan{}, ErrManifest
	}
	if err = ctx.Err(); err != nil {
		return recoveryInstallPlan{}, err
	}
	return plan, nil
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
