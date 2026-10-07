//go:build linux

package supervisor

import (
	"context"
	"os"
	"strings"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"golang.org/x/sys/unix"
)

// transferVolumeToGuest requires a pinned, qualified volume, independently
// reserved UID, committed ownership intent and retained stopped-runtime barrier.
// The caller must authenticate the intent against the durable store and retain
// the stopped-runtime barrier. This operation checks the exact inode but does
// not establish those publication prerequisites.
// Uncertain effects are preserved and exact owner/group retries are idempotent.
func transferVolumeToGuest(ctx context.Context, file *os.File, intent VolumeOwnershipIntent) error {
	size, uid, gid := intent.Size, intent.UID, intent.GID
	if err := ctx.Err(); err != nil {
		return err
	}
	if !guestproto.ValidID(intent.InstanceID) || len(intent.ImageSHA256) != 64 || strings.Trim(intent.ImageSHA256, "0123456789abcdef") != "" || intent.Inode == 0 || size < 16<<20 || size > 512<<30 || os.Geteuid() != 0 || file == nil || uid < 65536 || uid > 1<<31-1 || gid == 0 || gid > 1<<31-1 {
		return ErrPolicy
	}
	var before unix.Stat_t
	if unix.Fstat(int(file.Fd()), &before) != nil {
		return ErrPolicy
	}
	if uint64(before.Dev) != intent.Device || before.Ino != intent.Inode {
		return ErrPolicy
	}
	if before.Uid != 0 && before.Uid != uid {
		return ErrPolicy
	}
	if before.Uid == 0 && before.Gid != 0 {
		return ErrPolicy
	}
	if before.Uid == uid && before.Gid != gid {
		return ErrPolicy
	}
	if err := admitVolumeForUID(file, size, before.Uid); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if before.Uid == 0 {
		if err := file.Chown(int(uid), int(gid)); err != nil {
			return err
		}
	}
	if err := file.Sync(); err != nil {
		return err
	}
	var after unix.Stat_t
	if unix.Fstat(int(file.Fd()), &after) != nil || after.Uid != uid || after.Gid != gid || before.Dev != after.Dev || before.Ino != after.Ino {
		return ErrPolicy
	}
	if err := admitVolumeForUID(file, size, uid); err != nil {
		return err
	}
	return ctx.Err()
}
