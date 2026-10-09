//go:build linux

package install

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"strings"

	"github.com/pranavreddyg17/home-node/internal/catalog"
	"golang.org/x/sys/unix"
)

type guestStorageImageIdentity struct {
	OwnerID             string `json:"ownerId"`
	SHA256              string `json:"sha256"`
	Bytes               int64  `json:"bytes"`
	SourceGID, GuestGID uint32
	Device, Inode       uint64
}

// Caller authenticates the catalog, installed source group and retained image
// pathname under migration exclusion. This derives descriptor identity only;
// it grants no ownership mutation or publication authority.
func qualifyGuestStorageImage(ctx context.Context, plan GuestStorageProvisioningPlan, image catalog.Image, sourceGID uint32, file *os.File) (guestStorageImageIdentity, error) {
	if err := ctx.Err(); err != nil {
		return guestStorageImageIdentity{}, err
	}
	if _, err := canonicalGuestStoragePlan(ctx, plan); err != nil {
		return guestStorageImageIdentity{}, err
	}
	if os.Geteuid() != 0 || file == nil || sourceGID == 0 || sourceGID > 1<<31-1 || image.Bytes < 1 || image.Bytes > 64<<30 || len(image.SHA256) != 64 || strings.Trim(image.SHA256, "0123456789abcdef") != "" {
		return guestStorageImageIdentity{}, ErrConflict
	}
	flags, err := unix.FcntlInt(file.Fd(), unix.F_GETFL, 0)
	if err != nil || flags&unix.O_ACCMODE != unix.O_RDONLY {
		return guestStorageImageIdentity{}, ErrConflict
	}
	var before unix.Stat_t
	if unix.Fstat(int(file.Fd()), &before) != nil || before.Mode != unix.S_IFREG|0440 || before.Uid != 0 || before.Gid != sourceGID || before.Nlink != 1 || before.Size != image.Bytes {
		return guestStorageImageIdentity{}, ErrConflict
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return guestStorageImageIdentity{}, err
	}
	h := sha256.New()
	n, err := io.Copy(h, contextReader{ctx, io.LimitReader(file, image.Bytes+1)})
	if err != nil {
		return guestStorageImageIdentity{}, err
	}
	if n != image.Bytes || hex.EncodeToString(h.Sum(nil)) != image.SHA256 {
		return guestStorageImageIdentity{}, catalog.ErrUntrusted
	}
	var after unix.Stat_t
	if unix.Fstat(int(file.Fd()), &after) != nil || after.Dev != before.Dev || after.Ino != before.Ino || after.Mode != before.Mode || after.Uid != before.Uid || after.Gid != before.Gid || after.Nlink != before.Nlink || after.Size != before.Size {
		return guestStorageImageIdentity{}, ErrConflict
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return guestStorageImageIdentity{}, err
	}
	if err := ctx.Err(); err != nil {
		return guestStorageImageIdentity{}, err
	}
	return guestStorageImageIdentity{OwnerID: plan.Identity.OwnerID, SHA256: image.SHA256, Bytes: image.Bytes, SourceGID: sourceGID, GuestGID: plan.GuestGID, Device: uint64(before.Dev), Inode: before.Ino}, nil
}
