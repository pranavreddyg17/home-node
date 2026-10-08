package install

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// Verify a retained descriptor without opening a mutable pathname or changing
// its offset. Caller retains exclusion from writers through publication.
func verifyRecoveryPublicationDescriptor(ctx context.Context, file *os.File, intent recoveryPublicationIntent) error {
	return verifyRecoveryPublicationContent(ctx, file, intent, false)
}

func verifyRecoveryPublicationContent(ctx context.Context, file *os.File, intent recoveryPublicationIntent, rootStaging bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	validation := intent
	if rootStaging {
		if intent.Identity.UID != 0 || intent.Identity.GID != 0 {
			return ErrPlan
		}
		validation.Identity.UID, validation.Identity.GID = 1, 1
	}
	if file == nil || validateRecoveryPublicationIntent(validation) != nil {
		return ErrPlan
	}
	admitted := func() bool {
		var stat unix.Stat_t
		i := intent.Identity
		return unix.Fstat(int(file.Fd()), &stat) == nil && stat.Mode == unix.S_IFREG|0600 && stat.Nlink == 1 && uint64(stat.Dev) == i.Device && stat.Ino == i.Inode && stat.Size == i.Bytes && stat.Uid == i.UID && stat.Gid == i.GID
	}
	if !admitted() {
		return ErrConflict
	}
	hash := sha256.New()
	n, err := io.Copy(hash, contextReader{ctx, io.NewSectionReader(file, 0, intent.Identity.Bytes)})
	if err != nil {
		return err
	}
	if n != intent.Identity.Bytes || hex.EncodeToString(hash.Sum(nil)) != intent.ContentSHA256 || !admitted() {
		return ErrConflict
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if !admitted() {
		return ErrConflict
	}
	return ctx.Err()
}
