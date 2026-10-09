//go:build linux

package install

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// Retain the recorded replacements while both original configurations remain
// installed. The consumer may inspect or prepare publication; it must leave
// these namespaces intact. Publication needs its own interrupted-state checks.
func (e *Engine) withGuestStorageConfigurationStaged(ctx context.Context, current journal, plan GuestStorageProvisioningPlan, guard func(context.Context) error, use func(guestStorageConfigurationStage, []*os.File, func() error) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if guard == nil || use == nil {
		return ErrPlan
	}
	return e.withGuestStorageConfigurationSources(ctx, current, plan, guard, func(ctx context.Context, intent guestStorageConfigurationIntent, policy, environment *os.File, checkSources func() error) (result error) {
		const receiptName = "guest-storage-configuration-stage.json"
		record, err := e.journalRoot.OpenFile(receiptName, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
		if err != nil {
			return err
		}
		defer func() { result = errors.Join(result, record.Close()) }()
		recordInfo, err := record.Stat()
		if err != nil || !accountJournalFileAdmitted(recordInfo, e.owner, 8192) || !e.accountJournalPathUnchanged(receiptName, recordInfo, 8192) {
			return ErrConflict
		}
		stage, err := e.loadGuestStorageConfigurationStage(ctx, current, plan)
		if err != nil {
			return err
		}
		directory, err := e.host.OpenRoot("etc/homenode")
		if err != nil {
			return err
		}
		defer func() { result = errors.Join(result, directory.Close()) }()
		parent, err := directory.Open(".")
		if err != nil {
			return err
		}
		defer func() { result = errors.Join(result, parent.Close()) }()
		files := make([]*os.File, 0, 2)
		defer func() {
			for _, file := range files {
				result = errors.Join(result, file.Close())
			}
		}()
		sources := []*os.File{policy, environment}
		for i, receipt := range stage.Files {
			var source unix.Stat_t
			if unix.Fstat(int(sources[i].Fd()), &source) != nil || uint64(source.Dev) != receipt.SourceDevice || source.Ino != receipt.SourceInode {
				return ErrConflict
			}
			file, err := directory.OpenFile(receipt.Name, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
			if err != nil {
				return err
			}
			files = append(files, file)
		}
		payloads := [][]byte{intent.Policy, intent.Environment}
		check := func() error {
			if err := checkSources(); err != nil {
				return err
			}
			currentRecord, err := record.Stat()
			if err != nil || !accountJournalFileAdmitted(currentRecord, e.owner, 8192) || !os.SameFile(recordInfo, currentRecord) || currentRecord.Size() != recordInfo.Size() || !e.accountJournalPathUnchanged(receiptName, recordInfo, 8192) {
				return ErrConflict
			}
			loaded, err := e.loadGuestStorageConfigurationStage(ctx, current, plan)
			if err != nil {
				return err
			}
			// The receipt remains independently authenticated on every guard.
			for i, receipt := range stage.Files {
				if loaded.Files[i] != receipt {
					return ErrConflict
				}
				var observed, named unix.Stat_t
				file := files[i]
				if unix.Fstat(int(file.Fd()), &observed) != nil || unix.Fstatat(int(parent.Fd()), receipt.Name, &named, unix.AT_SYMLINK_NOFOLLOW) != nil || uint64(observed.Dev) != receipt.Device || observed.Ino != receipt.Inode || observed.Mode != unix.S_IFREG|0600 || observed.Uid != 0 || observed.Gid != 0 || observed.Nlink != 1 || observed.Size != receipt.Bytes || named.Dev != observed.Dev || named.Ino != observed.Ino {
					return ErrConflict
				}
				for _, attribute := range []string{"system.posix_acl_access", "system.posix_acl_default"} {
					if _, err := unix.Fgetxattr(int(file.Fd()), attribute, nil); !errors.Is(err, unix.ENODATA) {
						return ErrConflict
					}
				}
				data, err := io.ReadAll(io.NewSectionReader(file, 0, receipt.Bytes+1))
				if err != nil {
					return err
				}
				if !bytes.Equal(data, payloads[i]) {
					return ErrConflict
				}
				var parentMount, fileMount, namedMount unix.Statx_t
				flags := unix.AT_EMPTY_PATH | unix.AT_SYMLINK_NOFOLLOW
				if unix.Statx(int(parent.Fd()), "", flags, unix.STATX_MNT_ID, &parentMount) != nil || unix.Statx(int(file.Fd()), "", flags, unix.STATX_MNT_ID, &fileMount) != nil || unix.Statx(int(parent.Fd()), receipt.Name, unix.AT_SYMLINK_NOFOLLOW, unix.STATX_MNT_ID, &namedMount) != nil || parentMount.Mask&unix.STATX_MNT_ID == 0 || fileMount.Mask&unix.STATX_MNT_ID == 0 || namedMount.Mask&unix.STATX_MNT_ID == 0 || parentMount.Mnt_id == 0 || parentMount.Mnt_id != fileMount.Mnt_id || fileMount.Mnt_id != namedMount.Mnt_id {
					return ErrConflict
				}
			}
			opened, err := parent.Stat()
			named, nameErr := e.host.Lstat("etc/homenode")
			if err != nil || nameErr != nil || !os.SameFile(opened, named) {
				return ErrConflict
			}
			return checkSources()
		}
		if err := check(); err != nil {
			return err
		}
		if err := use(stage, files, check); err != nil {
			return err
		}
		return check()
	})
}
