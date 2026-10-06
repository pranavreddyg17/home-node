package install

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"
	"syscall"

	"github.com/pranavreddyg17/home-node/internal/backup"
	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"golang.org/x/sys/unix"
)

// copyRecoveryFile requires committed matching recovery intent and retained
// WithFiles scope. Output is private installer-owned staging, never a live
// controller/volume directory. Uncertain partial bytes remain for explicit repair.
func copyRecoveryFile(ctx context.Context, output *os.Root, stage string, source backup.PreparedRecoveryFile, owner int) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if output == nil || source.File == nil || source.Bytes <= 0 || source.Bytes > 512<<30 || len(source.SHA256) != 64 || !validRecoveryCopyStage(stage) {
		return ErrPlan
	}
	if decoded, err := hex.DecodeString(source.SHA256); err != nil || len(decoded) != 32 {
		return ErrPlan
	}
	directory, err := output.Stat(".")
	if err != nil || !directory.IsDir() || !owned(directory, owner) || directory.Mode().Perm() != 0700 {
		return ErrConflict
	}
	before, err := source.File.Stat()
	if err != nil || !before.Mode().IsRegular() || before.Size() != source.Bytes {
		return ErrConflict
	}
	out, err := output.OpenFile(stage, os.O_CREATE|os.O_EXCL|os.O_WRONLY|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, out.Close()) }()
	writer := &recoveryCopyWriter{file: out, remaining: source.Bytes}
	if err = writer.space(); err != nil {
		return err
	}
	hash := sha256.New()
	// ReadAt leaves the borrowed descriptor offset untouched and prevents reads
	// beyond the inventory size. The source descriptor itself must stay pinned.
	n, err := io.Copy(io.MultiWriter(writer, hash), contextReader{ctx, io.NewSectionReader(source.File, 0, source.Bytes)})
	if err != nil {
		return err
	}
	after, err := source.File.Stat()
	if err != nil || !os.SameFile(before, after) || after.Size() != source.Bytes || n != source.Bytes || hex.EncodeToString(hash.Sum(nil)) != source.SHA256 {
		return ErrConflict
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = out.Sync(); err != nil {
		return err
	}
	if err = syncDirectory(output, "."); err != nil {
		return err
	}
	return ctx.Err()
}

// Recheck the filesystem at each write so concurrent consumption cannot bypass
// the reserve established by replacement-host configuration admission.
type recoveryCopyWriter struct {
	file      *os.File
	remaining int64
}

func (w *recoveryCopyWriter) space() error {
	var stat unix.Statfs_t
	if unix.Fstatfs(int(w.file.Fd()), &stat) != nil || stat.Bsize <= 0 || w.remaining < 0 {
		return ErrConflict
	}
	required := uint64(w.remaining) + 4*(1<<30)
	block := uint64(stat.Bsize)
	blocks := required / block
	if required%block != 0 {
		blocks++
	}
	if stat.Bavail < blocks {
		return ErrConflict
	}
	return nil
}
func (w *recoveryCopyWriter) Write(data []byte) (int, error) {
	if int64(len(data)) > w.remaining {
		return 0, ErrConflict
	}
	if err := w.space(); err != nil {
		return 0, err
	}
	n, err := w.file.Write(data)
	w.remaining -= int64(n)
	return n, err
}

func validRecoveryCopyStage(stage string) bool {
	if stage == ".recovery-management.copy" {
		return true
	}
	id := strings.TrimSuffix(strings.TrimPrefix(stage, ".recovery-"), ".copy")
	return stage == ".recovery-"+id+".copy" && guestproto.ValidID(id)
}
