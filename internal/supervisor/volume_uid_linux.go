//go:build linux

package supervisor

import (
	"context"
	"golang.org/x/sys/unix"
	"os"
)

// transferVolumeToGuest requires a pinned, qualified volume, independently
// reserved UID, committed ownership intent and retained stopped-runtime barrier.
// This metadata operation does not establish those publication prerequisites.
// Uncertain effects are preserved and exact owner/group retries are idempotent.
func transferVolumeToGuest(ctx context.Context, file *os.File, size int64, uid, gid uint32) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if os.Geteuid() != 0 || file == nil || uid < 65536 || uid > 1<<31-1 || gid == 0 || gid > 1<<31-1 {
		return ErrPolicy
	}
	var before unix.Stat_t
	if unix.Fstat(int(file.Fd()), &before) != nil {
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
