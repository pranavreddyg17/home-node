package install

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"syscall"

	"github.com/pranavreddyg17/home-node/internal/backup"
)

// verifyRecoveryCopy requires matching immutable intent and a private retained
// destination root. It rehashes completed staging without modifying it; partial,
// foreign, permissive or replaced objects block retry/publication.
func verifyRecoveryCopy(ctx context.Context, root *os.Root, stage string, expected backup.PreparedRecoveryFile, owner int) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if root == nil || !validRecoveryCopyStage(stage) || expected.Bytes <= 0 || expected.Bytes > 512<<30 || len(expected.SHA256) != 64 {
		return ErrPlan
	}
	if _, err := hex.DecodeString(expected.SHA256); err != nil {
		return ErrPlan
	}
	directory, err := root.Stat(".")
	if err != nil || !directory.IsDir() || !owned(directory, owner) || directory.Mode().Perm() != 0700 {
		return ErrConflict
	}
	before, err := root.Lstat(stage)
	if err != nil {
		return err
	}
	if !before.Mode().IsRegular() || !owned(before, owner) || before.Mode().Perm() != 0600 || before.Size() != expected.Bytes {
		return ErrConflict
	}
	file, err := root.OpenFile(stage, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, file.Close()) }()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return ErrConflict
	}
	hash := sha256.New()
	n, err := io.Copy(hash, io.LimitReader(contextReader{ctx, file}, expected.Bytes+1))
	if err != nil {
		return err
	}
	if n != expected.Bytes || hex.EncodeToString(hash.Sum(nil)) != expected.SHA256 {
		return ErrConflict
	}
	current, err := root.Lstat(stage)
	if err != nil || !os.SameFile(opened, current) || !current.Mode().IsRegular() || current.Size() != expected.Bytes || current.Mode().Perm() != 0600 || !owned(current, owner) {
		return ErrConflict
	}
	return ctx.Err()
}
