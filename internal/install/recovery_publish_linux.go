//go:build linux

package install

import (
	"context"
	"os"
	"strings"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"golang.org/x/sys/unix"
)

// publishRecoveryWithIntent composes records, content and the namespace step.
// Caller holds e.mu, the qualified destination/file descriptors and the retained
// activation-marker scope. The guard must reobserve loaded units and guests.
// This private boundary does not enable production recovery orchestration.
func (e *Engine) publishRecoveryWithIntent(ctx context.Context, directory, file *os.File, stage string, intent recoveryPublicationIntent, recovery recoveryIntent, guard func(context.Context) error) error {
	if guard == nil {
		return ErrPlan
	}
	return e.withRecoveryPublicationIntent(ctx, intent, recovery, func(ctx context.Context) error {
		if err := guard(ctx); err != nil {
			return err
		}
		if err := verifyRecoveryPublicationDescriptor(ctx, file, intent); err != nil {
			return err
		}
		if err := guard(ctx); err != nil {
			return err
		}
		if err := publishRecoveryFile(ctx, directory, stage, intent.FileName, intent.Identity); err != nil {
			return err
		}
		if err := verifyRecoveryPublicationDescriptor(ctx, file, intent); err != nil {
			return err
		}
		return guard(ctx)
	})
}

// publishRecoveryFile performs only the final no-replacement namespace step.
// Caller retains the qualified directory, runtime exclusion, source/content
// verification and durable ownership intent through this call and reconciliation.
// Failure after rename preserves the file for exact journaled retry.
func publishRecoveryFile(ctx context.Context, directory *os.File, stage, final string, expected recoveryPublicationIdentity) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if directory == nil || expected.Inode == 0 || expected.Bytes <= 0 || expected.Bytes > 512<<30 {
		return ErrPlan
	}
	id := strings.TrimSuffix(final, ".raw")
	if final == "management.db" {
		if stage != ".recovery-management.publish" {
			return ErrPlan
		}
	} else if !guestproto.ValidID(id) || final != id+".raw" || stage != ".recovery-"+id+".publish" {
		return ErrPlan
	}
	fd := int(directory.Fd())
	var parent unix.Stat_t
	if unix.Fstat(fd, &parent) != nil || parent.Mode&unix.S_IFMT != unix.S_IFDIR {
		return ErrConflict
	}
	admitted := func(name string) bool {
		var stat unix.Stat_t
		return unix.Fstatat(fd, name, &stat, unix.AT_SYMLINK_NOFOLLOW) == nil && stat.Mode == unix.S_IFREG|0600 && stat.Nlink == 1 && uint64(stat.Dev) == expected.Device && stat.Ino == expected.Inode && stat.Size == expected.Bytes && stat.Uid == expected.UID && stat.Gid == expected.GID
	}
	if !admitted(stage) {
		// An interrupted rename can leave only the final name. The immutable
		// journaled inode, not matching content alone, authorizes this retry.
		var missing unix.Stat_t
		if unix.Fstatat(fd, stage, &missing, unix.AT_SYMLINK_NOFOLLOW) == unix.ENOENT && admitted(final) {
			if err := directory.Sync(); err != nil {
				return err
			}
			if !admitted(final) || unix.Fstatat(fd, stage, &missing, unix.AT_SYMLINK_NOFOLLOW) != unix.ENOENT {
				return ErrConflict
			}
			return ctx.Err()
		}
		return ErrConflict
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := unix.Renameat2(fd, stage, fd, final, unix.RENAME_NOREPLACE); err != nil {
		return err
	}
	if err := directory.Sync(); err != nil {
		return err
	}
	if !admitted(final) {
		return ErrConflict
	}
	return ctx.Err()
}
