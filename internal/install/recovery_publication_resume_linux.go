//go:build linux

package install

import (
	"context"
	"errors"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// Resume only the inode already committed before ownership transfer. Caller
// retains installation, destination and activation exclusion and independently
// derives the expected intent; neither existing bytes nor a pathname grant it.
func (e *Engine) withResumedRecoveryPublication(ctx context.Context, destination *os.Root, intent recoveryPublicationIntent, recovery recoveryIntent, guard func(context.Context) error, use func(context.Context, *os.File) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if destination == nil || guard == nil || use == nil || validateRecoveryPublicationIntent(intent) != nil {
		return ErrPlan
	}
	return e.withRecoveryPublicationIntent(ctx, intent, recovery, func(ctx context.Context) (result error) {
		if err := guard(ctx); err != nil {
			return err
		}
		stage := ".recovery-management.publish"
		if intent.FileName != "management.db" {
			stage = ".recovery-" + intent.FileName[:len(intent.FileName)-4] + ".publish"
		}
		name := stage
		file, err := destination.OpenFile(name, os.O_RDWR|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		published := false
		if errors.Is(err, os.ErrNotExist) {
			name, published = intent.FileName, true
			file, err = destination.OpenFile(name, os.O_RDWR|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		}
		if err != nil {
			return err
		}
		defer func() { result = errors.Join(result, file.Close()) }()
		var stat unix.Stat_t
		i := intent.Identity
		if unix.Fstat(int(file.Fd()), &stat) != nil || stat.Mode != unix.S_IFREG|0600 || stat.Nlink != 1 || uint64(stat.Dev) != i.Device || stat.Ino != i.Inode || stat.Size != i.Bytes {
			return ErrConflict
		}
		// chown changes the UID and GID together. Accept only the original
		// private root ownership or the complete committed target ownership.
		original := stat.Uid == 0 && stat.Gid == 0
		if published && original {
			return ErrConflict
		}
		if !original && (stat.Uid != i.UID || stat.Gid != i.GID) {
			return ErrConflict
		}
		observed := intent
		observed.Identity.UID, observed.Identity.GID = stat.Uid, stat.Gid
		// The descriptor verifier requires nonzero target IDs. Verify root
		// staging with the same hash and metadata checks before chown below.
		if original {
			if err := verifyRecoveryPublicationContent(ctx, file, observed, true); err != nil {
				return err
			}
		} else if err := verifyRecoveryPublicationDescriptor(ctx, file, intent); err != nil {
			return err
		}
		qualifyPath := func() error {
			current, err := destination.Lstat(name)
			opened, statErr := file.Stat()
			if err != nil || statErr != nil || !os.SameFile(current, opened) {
				return ErrConflict
			}
			if published {
				if _, err := destination.Lstat(stage); !errors.Is(err, os.ErrNotExist) {
					return ErrConflict
				}
			} else if _, err := destination.Lstat(intent.FileName); !errors.Is(err, os.ErrNotExist) {
				return ErrConflict
			}
			return nil
		}
		if err := guard(ctx); err != nil {
			return err
		}
		if err := qualifyPath(); err != nil {
			return err
		}
		if original {
			if err := file.Chown(int(i.UID), int(i.GID)); err != nil {
				return err
			}
		}
		if err := verifyRecoveryPublicationDescriptor(ctx, file, intent); err != nil {
			return err
		}
		if err := guard(ctx); err != nil {
			return err
		}
		if err := qualifyPath(); err != nil {
			return err
		}
		if err := use(ctx, file); err != nil {
			return err
		}
		if err := verifyRecoveryPublicationDescriptor(ctx, file, intent); err != nil {
			return err
		}
		return guard(ctx)
	})
}
