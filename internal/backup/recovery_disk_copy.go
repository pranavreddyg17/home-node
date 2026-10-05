package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
)

// copyRecoveryDisk creates a durable private staging file for later journaled
// publication. It does not qualify a filesystem or authorize installation.
// Callers must retain exclusive ownership of both roots and journal the target
// name and qualified inventory before calling; existing targets are refused.
func copyRecoveryDisk(ctx context.Context, source, destination *os.Root, disk RecoveryInstallDisk, target string) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if source == nil || destination == nil || (disk.Workload != "files" && disk.Workload != "ai") || disk.SourceName != disk.Workload+".raw" || disk.Bytes <= 0 || disk.Bytes > 512<<30 || !repositoryPattern.MatchString(disk.SourceSHA256) || target != ".recovery-"+disk.InstanceID+".stage" || !guestproto.ValidID(disk.InstanceID) {
		return ErrManifest
	}
	return copyRecoveryPayload(ctx, source, destination, BackupFile{Name: disk.SourceName, Bytes: disk.Bytes, SHA256: disk.SourceSHA256}, target)
}

// copyRecoveryPayload is shared byte copying only, without compatibility,
// filesystem or runtime authority. Callers validate the typed source and target.
func copyRecoveryPayload(ctx context.Context, source, destination *os.Root, entry BackupFile, target string) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if source == nil || destination == nil || entry.Name == "" || path.Base(entry.Name) != entry.Name || entry.Name == "." || entry.Name == ".." || target == "" || path.Base(target) != target || target == "." || target == ".." || entry.Bytes <= 0 || entry.Bytes > 512<<30 || !repositoryPattern.MatchString(entry.SHA256) {
		return ErrManifest
	}
	for _, root := range []*os.Root{source, destination} {
		info, err := root.Stat(".")
		if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
			return ErrManifest
		}
	}
	expected, err := source.Lstat(entry.Name)
	if err != nil || !expected.Mode().IsRegular() || expected.Size() != entry.Bytes {
		return ErrManifest
	}
	input, err := source.Open(entry.Name)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, input.Close()) }()
	info, err := input.Stat()
	if err != nil || !os.SameFile(expected, info) {
		return ErrManifest
	}
	directory, err := destination.Open(".")
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, directory.Close()) }()
	if err = requireStagingSpace(directory, entry.Bytes); err != nil {
		return err
	}
	output, err := destination.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	created, err := output.Stat()
	if err != nil {
		return errors.Join(err, output.Close())
	}
	defer func() {
		result = errors.Join(result, output.Close())
		if result != nil {
			result = errors.Join(result, removeOwnedStaging(destination, map[string]os.FileInfo{target: created}), directory.Sync())
		}
	}()
	hash := sha256.New()
	writer := &restoreWriter{ctx: ctx, stage: directory, destination: io.MultiWriter(output, hash), remaining: entry.Bytes}
	if _, err = io.CopyBuffer(writer, io.LimitReader(input, entry.Bytes), make([]byte, 1<<20)); err != nil {
		return err
	}
	var extra [1]byte
	if n, readErr := input.Read(extra[:]); writer.remaining != 0 || n != 0 || readErr != io.EOF || hex.EncodeToString(hash.Sum(nil)) != entry.SHA256 {
		return ErrManifest
	}
	current, err := source.Lstat(entry.Name)
	if err != nil || !os.SameFile(info, current) || current.Size() != entry.Bytes {
		return ErrManifest
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = output.Sync(); err != nil {
		return err
	}
	return errors.Join(directory.Sync(), ctx.Err())
}
