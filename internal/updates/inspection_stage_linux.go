//go:build linux

package updates

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// StageInspectionPackage publishes a private copy and durable operation intent.
// It neither activates inspection nor approves installation. The root caller
// must obtain staging from verified installer ownership and release from its
// signed acquisition operation. Existing/interrupted state requires recovery.
func StageInspectionPackage(ctx context.Context, staging *os.Root, release *AcquiredRelease, operation string) error {
	if os.Geteuid() != 0 {
		return ErrPackageArchive
	}
	return stageInspectionPackageOwned(ctx, staging, release, operation)
}

func stageInspectionPackageOwned(ctx context.Context, staging *os.Root, release *AcquiredRelease, operation string) (resultErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if staging == nil || release == nil || release.Package == nil || !inspectionOperation.MatchString(operation) {
		return ErrPackageArchive
	}
	identity := InspectionIdentity{OperationID: operation, Release: release.Metadata.Release, PackageSHA256: release.PackageSHA256, PackageLength: release.PackageLength}
	if !validInspectionIdentity(identity) {
		return ErrInspectionResult
	}
	directory, err := staging.Open(".")
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, directory.Close()) }()
	info, err := directory.Stat()
	var native unix.Stat_t
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 || unix.Fstat(int(directory.Fd()), &native) != nil || native.Uid != uint32(os.Geteuid()) || native.Gid != uint32(os.Getegid()) {
		return ErrPackageArchive
	}
	if err = unix.Flock(int(directory.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return err
	}
	// Directory inode ownership is held until durable ready publication.
	entries, readErr := directory.ReadDir(1)
	if len(entries) != 0 || readErr != nil && !errors.Is(readErr, io.EOF) {
		return ErrPackageArchive
	}
	flags, err := unix.FcntlInt(release.Package.Fd(), unix.F_GETFL, 0)
	if err != nil || flags&unix.O_ACCMODE != unix.O_RDONLY {
		return ErrPackageArchive
	}
	info, err = release.Package.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0400 || unix.Fstat(int(release.Package.Fd()), &native) != nil || native.Uid != uint32(os.Geteuid()) || native.Nlink != 1 {
		return ErrPackageArchive
	}
	digest, length, err := PackageIdentity(ctx, release.Package)
	if err != nil || digest != identity.PackageSHA256 || length != identity.PackageLength {
		return errors.Join(ErrPackageArchive, err)
	}
	var space unix.Statfs_t
	if unix.Fstatfs(int(directory.Fd()), &space) != nil || space.Bsize <= 0 {
		return ErrPackageArchive
	}
	required := uint64(length) + packageDiskReserve
	block := uint64(space.Bsize)
	if space.Bavail < (required+block-1)/block {
		return ErrPackageArchive
	}
	intent, err := json.Marshal(struct {
		Schema      int    `json:"schema"`
		OperationID string `json:"operationId"`
		Release     string `json:"release"`
		SHA256      string `json:"sha256"`
		Length      int64  `json:"length"`
	}{1, operation, identity.Release, digest, length})
	if err != nil {
		return err
	}
	if err = writeInitializationFile(staging, "intent", intent); err != nil {
		return err
	}
	file, err := staging.OpenFile("package.pending", os.O_CREATE|os.O_EXCL|os.O_WRONLY|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed {
			resultErr = errors.Join(resultErr, file.Close())
		}
	}()
	hash := sha256.New()
	count, err := io.Copy(io.MultiWriter(file, hash), contextualReader{ctx, io.NewSectionReader(release.Package, 0, length)})
	if err != nil || count != length || hex.EncodeToString(hash.Sum(nil)) != digest {
		return errors.Join(ErrPackageArchive, err)
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = file.Chmod(0400); err != nil {
		return err
	}
	if err = errors.Join(file.Sync(), file.Close()); err != nil {
		closed = true
		return err
	}
	closed = true
	if err = unix.Renameat2(int(directory.Fd()), "package.pending", int(directory.Fd()), "package.deb", unix.RENAME_NOREPLACE); err != nil {
		return err
	}
	if err = directory.Sync(); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return errors.Join(writeInitializationFile(staging, "ready", intent), ctx.Err())
}
