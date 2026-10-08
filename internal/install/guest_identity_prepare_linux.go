//go:build linux

package install

import (
	"context"
	"errors"
	"io"
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func (e *Engine) PrepareGuestIdentityConfiguration(ctx context.Context) (preview GuestIdentityConfigurationPreview, result error) {
	if err := ctx.Err(); err != nil {
		return preview, err
	}
	if os.Geteuid() != 0 || e.host.Name() != "/" {
		return preview, ErrAccounts
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if !e.mu.TryLock() {
		return preview, ErrConflict
	}
	defer e.mu.Unlock()
	owner, err := e.inspectGuestIdentityAccountsLocked(ctx)
	if err != nil {
		return preview, err
	}
	directory, err := e.host.OpenRoot("etc")
	if err != nil {
		return preview, err
	}
	defer func() {
		result = errors.Join(result, directory.Close())
		if result != nil {
			preview = GuestIdentityConfigurationPreview{}
		}
	}()
	parent, err := directory.Open(".")
	if err != nil {
		return preview, err
	}
	defer func() {
		result = errors.Join(result, parent.Close())
		if result != nil {
			preview = GuestIdentityConfigurationPreview{}
		}
	}()
	var parentStat unix.Stat_t
	if unix.Fstat(int(parent.Fd()), &parentStat) != nil || parentStat.Mode&unix.S_IFMT != unix.S_IFDIR || parentStat.Uid != 0 || parentStat.Gid != 0 || parentStat.Mode&0022 != 0 {
		return preview, ErrConflict
	}
	file, err := directory.OpenFile("nsswitch.conf", os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return preview, err
	}
	defer func() {
		result = errors.Join(result, file.Close())
		if result != nil {
			preview = GuestIdentityConfigurationPreview{}
		}
	}()
	var initial unix.Stat_t
	if unix.Fstat(int(file.Fd()), &initial) != nil || initial.Mode != unix.S_IFREG|0644 || initial.Nlink != 1 || initial.Uid != 0 || initial.Gid != 0 || initial.Size <= 0 || initial.Size > 8192 {
		return preview, ErrConflict
	}
	data, err := io.ReadAll(io.LimitReader(file, 8193))
	if err != nil {
		return preview, err
	}
	proposal, err := planGuestIdentityNameServices(data)
	if err != nil {
		return preview, err
	}
	opened, statErr := file.Stat()
	named, nameErr := directory.Lstat("nsswitch.conf")
	parentOpened, parentErr := parent.Stat()
	parentNamed, parentNameErr := e.host.Lstat("etc")
	if statErr != nil || nameErr != nil || parentErr != nil || parentNameErr != nil || !os.SameFile(opened, named) || !os.SameFile(parentOpened, parentNamed) {
		return preview, ErrConflict
	}
	if err := e.commitGuestIdentityNameServices(ctx, owner, data, proposal); err != nil {
		return preview, err
	}
	intent := guestIdentityNameServiceIntent{Version: 1, OwnerID: owner, Original: string(data), Proposal: proposal}
	if err := e.withGuestIdentityHostSource(ctx, intent, func(ctx context.Context, _ *os.File) error {
		currentOwner, err := e.inspectGuestIdentityAccountsLocked(ctx)
		if err != nil {
			return err
		}
		if currentOwner != owner {
			return ErrConflict
		}
		return nil
	}); err != nil {
		return preview, err
	}
	return GuestIdentityConfigurationPreview{OwnerID: owner, OriginalSHA256: proposal.OriginalSHA256, DesiredSHA256: proposal.DesiredSHA256, ChangesRequired: proposal.OriginalSHA256 != proposal.DesiredSHA256, IntentCommitted: true}, nil
}
