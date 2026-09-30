package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
)

// StageDisk copies a descriptor held by the supervisor's disk lease into a
// private, exclusively owned staging root. It provides integrity and durable
// staging, not filesystem-consistency or lease authority. The caller must keep
// that lease for the complete call and qualify the source filesystem separately.
func StageDisk(ctx context.Context, root *os.Root, source *os.File, entry BackupFile) (result BackupFile, resultErr error) {
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if root == nil || source == nil || entry.Workload != "files" && entry.Workload != "ai" || entry.Name != entry.Workload+".raw" || entry.Bytes < 16<<20 || entry.Bytes > 512<<30 || entry.DataSchema != 1 || entry.Protocol != 1 || !repositoryPattern.MatchString(entry.ImageSHA256) || entry.SHA256 != "" {
		return result, ErrManifest
	}
	info, err := root.Stat(".")
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return result, ErrManifest
	}
	before, err := source.Stat()
	if err != nil || !before.Mode().IsRegular() || before.Size() != entry.Bytes {
		return result, ErrManifest
	}
	directory, err := root.Open(".")
	if err != nil {
		return result, err
	}
	defer directory.Close()
	file, err := root.OpenFile(entry.Name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return result, err
	}
	created, err := file.Stat()
	if err != nil {
		file.Close()
		return result, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, file.Close())
		if resultErr != nil {
			// Never remove a different inode substituted by another host writer.
			current, err := root.Lstat(entry.Name)
			if err == nil && os.SameFile(created, current) {
				resultErr = errors.Join(resultErr, root.Remove(entry.Name), directory.Sync())
			}
			result = BackupFile{}
		}
	}()
	hash := sha256.New()
	reader := io.NewSectionReader(source, 0, entry.Bytes)
	buffer := make([]byte, 1<<20)
	remaining := entry.Bytes
	for remaining > 0 {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		n, err := io.ReadFull(reader, buffer[:min(int64(len(buffer)), remaining)])
		if err != nil {
			return result, err
		}
		written, err := file.Write(buffer[:n])
		if err != nil {
			return result, err
		}
		if written != n {
			return result, io.ErrShortWrite
		}
		_, _ = hash.Write(buffer[:n])
		remaining -= int64(n)
	}
	after, err := source.Stat()
	if err != nil || !os.SameFile(before, after) || after.Size() != entry.Bytes || !before.ModTime().Equal(after.ModTime()) {
		return result, ErrManifest
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if err = file.Sync(); err != nil {
		return result, err
	}
	if err = directory.Sync(); err != nil {
		return result, err
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	entry.SHA256 = hex.EncodeToString(hash.Sum(nil))
	return entry, nil
}
