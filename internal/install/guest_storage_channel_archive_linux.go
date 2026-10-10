//go:build linux

package install

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// Caller retains qualified installed/runtime/account exclusion. Only a valid
// receipt from another kernel boot may be archived, and only while both channel
// locations are absent. Preserve its exact inode under a boot-specific name;
// never overwrite history or use a fresh directory as old receipt authority.
func (e *Engine) archivePreviousBootGuestStorageChannelStage(ctx context.Context, plan GuestStorageProvisioningPlan, transferGID uint32, guard func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if e == nil || e.journalRoot == nil || guard == nil {
		return ErrPlan
	}
	stage, err := e.loadGuestStorageChannelStage(ctx, plan, transferGID)
	if err != nil {
		return err
	}
	bootID, err := observeGuestStorageBootID(ctx)
	if err != nil {
		return err
	}
	if stage.BootID == bootID {
		return ErrConflict
	}
	encoded, err := json.Marshal(stage)
	if err != nil {
		return err
	}
	return e.withGuestStorageChannelRuntime(ctx, guard, func(runtime *os.Root, _ *os.File, checkRuntime func(context.Context) error) (result error) {
		const source = "guest-storage-channel-stage.json"
		archive := "guest-storage-channel-stage." + stage.BootID + ".json"
		file, err := e.journalRoot.OpenFile(source, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
		if err != nil {
			return err
		}
		defer func() { result = errors.Join(result, file.Close()) }()
		before, err := file.Stat()
		if err != nil || !accountJournalFileAdmitted(before, e.owner, 8192) {
			return ErrConflict
		}
		parent, err := e.journalRoot.Open(".")
		if err != nil {
			return err
		}
		defer func() { result = errors.Join(result, parent.Close()) }()
		moved := false
		check := func() error {
			if err := checkRuntime(ctx); err != nil {
				return err
			}
			currentBoot, err := observeGuestStorageBootID(ctx)
			if err != nil {
				return err
			}
			if currentBoot != bootID {
				return ErrConflict
			}
			for _, name := range []string{"guests", ".homenode-guests.stage"} {
				if _, err := runtime.Lstat(name); !os.IsNotExist(err) {
					return errors.Join(ErrConflict, err)
				}
			}
			name, absent := source, archive
			if moved {
				name, absent = archive, source
			}
			if !e.accountJournalPathUnchanged(name, before, 8192) {
				return ErrConflict
			}
			if _, err := e.journalRoot.Lstat(absent); !os.IsNotExist(err) {
				return errors.Join(ErrConflict, err)
			}
			current, err := file.Stat()
			if err != nil || !os.SameFile(before, current) || !accountJournalFileAdmitted(current, e.owner, 8192) {
				return ErrConflict
			}
			data, err := io.ReadAll(io.NewSectionReader(file, 0, 8193))
			if err != nil {
				return err
			}
			if !bytes.Equal(data, encoded) {
				return ErrConflict
			}
			return checkRuntime(ctx)
		}
		if err := check(); err != nil {
			return err
		}
		if err := unix.Renameat2(int(parent.Fd()), source, int(parent.Fd()), archive, unix.RENAME_NOREPLACE); err != nil {
			return err
		}
		moved = true
		if err := parent.Sync(); err != nil {
			return err
		}
		return check()
	})
}
