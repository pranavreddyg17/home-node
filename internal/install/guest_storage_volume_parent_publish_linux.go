//go:build linux

package install

import (
	"context"
	"errors"
	"io"
	"os"
	"reflect"

	"golang.org/x/sys/unix"
)

// Caller retains installer/account exclusion and an activation barrier whose
// admission permits only this recorded transition. This phase requires an empty
// volume directory; populated volume migration needs per-volume provenance.
func (e *Engine) publishEmptyGuestStorageVolumeParentLocked(ctx context.Context, plan GuestStorageProvisioningPlan, sourceGID uint32, guard func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if guard == nil || os.Geteuid() != 0 {
		return ErrPlan
	}
	current, err := e.load()
	if err != nil {
		return err
	}
	return e.withGuestStorageVolumeParentIntent(ctx, current, plan, sourceGID, func(intent guestStorageVolumeParentIntent, checkIntent func() error) error {
		retained := func(ctx context.Context) error {
			if err := checkIntent(); err != nil {
				return err
			}
			if err := guard(ctx); err != nil {
				return err
			}
			return checkIntent()
		}
		return e.withRecordedGuestStorageVolumeParent(ctx, intent, retained, func(root *os.Root, parent *os.File, checkPath func(context.Context) error) error {
			verify := func() (journal, unix.Stat_t, error) {
				empty := journal{}
				var st unix.Stat_t
				if err := checkPath(ctx); err != nil {
					return empty, st, err
				}
				emptyCheck, err := root.Open(".")
				if err != nil {
					return empty, st, err
				}
				entries, readErr := emptyCheck.ReadDir(1)
				closeErr := emptyCheck.Close()
				if len(entries) != 0 || !errors.Is(readErr, io.EOF) || closeErr != nil {
					return empty, st, ErrConflict
				}
				current, err := e.load()
				if err != nil {
					return empty, st, err
				}
				if !reflect.DeepEqual(current, intent.Original) && !reflect.DeepEqual(current, intent.Desired) {
					return empty, st, ErrConflict
				}
				if unix.Fstat(int(parent.Fd()), &st) != nil || uint64(st.Dev) != intent.Device || st.Ino != intent.Inode || st.Mode != unix.S_IFDIR|0710 || st.Uid != 0 || (st.Gid != intent.SourceGID && st.Gid != intent.Plan.GuestGID) || (reflect.DeepEqual(current, intent.Desired) && st.Gid != intent.Plan.GuestGID) {
					return empty, st, ErrConflict
				}
				for _, item := range current.Items {
					if item.Path == "var/lib/homenode/volumes" {
						continue
					}
					if err := e.matches(item); err != nil {
						return empty, st, ErrConflict
					}
				}
				if err := checkPath(ctx); err != nil {
					return empty, st, err
				}
				final, err := e.load()
				var after unix.Stat_t
				if err != nil || !reflect.DeepEqual(final, current) || unix.Fstat(int(parent.Fd()), &after) != nil || after.Dev != st.Dev || after.Ino != st.Ino || after.Mode != st.Mode || after.Uid != st.Uid || after.Gid != st.Gid {
					return empty, st, ErrConflict
				}
				return current, st, ctx.Err()
			}
			current, st, err := verify()
			if err != nil {
				return err
			}
			if st.Gid != intent.Plan.GuestGID {
				if err := unix.Fchown(int(parent.Fd()), 0, int(intent.Plan.GuestGID)); err != nil {
					return err
				}
			}
			if err := parent.Sync(); err != nil {
				return err
			}
			if e.checkpoint != nil {
				if err := e.checkpoint("guest-storage-volume-parent-ownership-migrated", "var/lib/homenode/volumes"); err != nil {
					return err
				}
			}
			current, st, err = verify()
			if err != nil {
				return err
			}
			if st.Gid != intent.Plan.GuestGID {
				return ErrConflict
			}
			if !reflect.DeepEqual(current, intent.Desired) {
				if err := e.save(intent.Desired); err != nil {
					return err
				}
			}
			if e.checkpoint != nil {
				if err := e.checkpoint("guest-storage-volume-parent-journal-published", "install.json"); err != nil {
					return err
				}
			}
			current, st, err = verify()
			if err != nil {
				return err
			}
			if !reflect.DeepEqual(current, intent.Desired) || st.Gid != intent.Plan.GuestGID {
				return ErrConflict
			}
			return ctx.Err()
		})
	})
}
