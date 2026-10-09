//go:build linux

package install

import (
	"context"
	"os"

	"github.com/pranavreddyg17/home-node/internal/catalog"
	"golang.org/x/sys/unix"
)

// Caller retains authenticated provenance, catalog, migration exclusion and
// pathname/mount authority. Retry accepts destination ownership only for the
// exact recorded inode and authenticated content. No pathname is reopened.
func migrateGuestStorageImage(ctx context.Context, plan GuestStorageProvisioningPlan, receipt guestStorageImageIdentity, file *os.File, check func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if check == nil || file == nil {
		return ErrPlan
	}
	if _, err := canonicalGuestStorageImagesIntent(ctx, plan, []guestStorageImageIdentity{receipt}); err != nil {
		return err
	}
	qualify := func() error {
		if err := check(ctx); err != nil {
			return err
		}
		var st unix.Stat_t
		if unix.Fstat(int(file.Fd()), &st) != nil || (st.Gid != receipt.SourceGID && st.Gid != receipt.GuestGID) {
			return ErrConflict
		}
		got, err := qualifyGuestStorageImage(ctx, plan, catalog.Image{SHA256: receipt.SHA256, Bytes: receipt.Bytes}, st.Gid, file)
		if err != nil {
			return err
		}
		// Source group is historical provenance, not the current inode group.
		got.SourceGID = receipt.SourceGID
		if got != receipt {
			return ErrConflict
		}
		return check(ctx)
	}
	if err := qualify(); err != nil {
		return err
	}
	if err := unix.Fchown(int(file.Fd()), 0, int(receipt.GuestGID)); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	return qualify()
}
