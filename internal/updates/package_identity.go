package updates

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
)

// PackageIdentity hashes one bounded regular descriptor without changing its
// offset. It identifies bytes; it does not prove signed authorization.
func PackageIdentity(ctx context.Context, file *os.File) (string, int64, error) {
	if err := ctx.Err(); err != nil {
		return "", 0, err
	}
	if file == nil {
		return "", 0, ErrPackageArchive
	}
	before, err := file.Stat()
	if err != nil || !before.Mode().IsRegular() || before.Size() < 1 || before.Size() > 512<<20 {
		return "", 0, ErrPackageArchive
	}
	hash := sha256.New()
	count, err := io.Copy(hash, contextualReader{ctx, io.NewSectionReader(file, 0, before.Size())})
	if err != nil || count != before.Size() {
		return "", 0, errors.Join(ErrPackageArchive, err)
	}
	after, err := file.Stat()
	if err != nil || !os.SameFile(before, after) || after.Size() != before.Size() || !before.ModTime().Equal(after.ModTime()) {
		return "", 0, ErrPackageArchive
	}
	if err = ctx.Err(); err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hash.Sum(nil)), count, nil
}
