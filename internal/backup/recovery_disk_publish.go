package backup

import (
	"context"
	"errors"
	"os"
)

// publishRecoveryDisk exposes a verified private stage under its journaled new
// identity, without granting UID/runtime/device authority. The caller must retain
// exclusive private-root ownership. The stage link is intentionally retained:
// after interruption, only the same inode can establish prior publication.
func publishRecoveryDisk(ctx context.Context, root *os.Root, disk RecoveryInstallDisk) (name string, result error) {
	plan, err := loadRecoveryInstallPlan(ctx, root)
	if err != nil {
		return "", err
	}
	matched := false
	for _, recorded := range plan.Disks {
		if recorded == disk {
			matched = true
		}
	}
	if !matched {
		return "", ErrManifest
	}
	stage := ".recovery-" + disk.InstanceID + ".stage"
	entry := BackupFile{Name: stage, Bytes: disk.Bytes, SHA256: disk.SourceSHA256}
	if err = verifyBackupFile(ctx, root, entry); err != nil {
		return "", err
	}
	expected, err := root.Lstat(stage)
	if err != nil || !expected.Mode().IsRegular() || expected.Mode().Perm() != 0600 {
		return "", ErrManifest
	}
	file, err := root.Open(stage)
	if err != nil {
		return "", err
	}
	opened, err := file.Stat()
	if err != nil || !os.SameFile(expected, opened) {
		return "", errors.Join(ErrManifest, file.Close())
	}
	qualification := QualifyExt4Disk(ctx, file)
	if err = errors.Join(qualification, file.Close()); err != nil {
		return "", err
	}
	current, err := root.Lstat(stage)
	if err != nil || !os.SameFile(opened, current) || current.Size() != disk.Bytes || current.Mode().Perm() != 0600 {
		return "", ErrManifest
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	final := disk.InstanceID + ".raw"
	if existing, statErr := root.Lstat(final); statErr == nil {
		if !existing.Mode().IsRegular() || !os.SameFile(current, existing) {
			return "", ErrManifest
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return "", statErr
	} else if err = root.Link(stage, final); err != nil {
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
	return final, nil
}
