//go:build linux

package install

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

type guestIdentityNameServiceStage struct {
	Version      int    `json:"version"`
	IntentSHA256 string `json:"intentSha256"`
	SourceDevice uint64 `json:"sourceDevice"`
	SourceInode  uint64 `json:"sourceInode"`
	Device       uint64 `json:"device"`
	Inode        uint64 `json:"inode"`
	Bytes        int64  `json:"bytes"`
}

// Caller retains qualified source, directory, intent and allocation exclusion.
// Stage under a fixed create-only name; never adopt matching foreign bytes.
// Partial stages remain preserved for explicit journal-based reconciliation.
func (e *Engine) stageGuestIdentityNameServices(ctx context.Context, directory *os.Root, source *os.File, intent guestIdentityNameServiceIntent, guard func(context.Context) error) (stage guestIdentityNameServiceStage, result error) {
	if err := ctx.Err(); err != nil {
		return stage, err
	}
	if directory == nil || source == nil || guard == nil {
		return stage, ErrPlan
	}
	result = e.withGuestIdentityNameServiceIntent(ctx, intent, func(ctx context.Context) (result error) {
		if err := guard(ctx); err != nil {
			return err
		}
		var original unix.Stat_t
		if unix.Fstat(int(source.Fd()), &original) != nil || original.Mode != unix.S_IFREG|0644 || original.Nlink != 1 || original.Uid != 0 || original.Gid != 0 || original.Size != int64(len(intent.Original)) {
			return ErrConflict
		}
		checkSource := func() error {
			if err := ctx.Err(); err != nil {
				return err
			}
			data, err := io.ReadAll(io.NewSectionReader(source, 0, original.Size+1))
			if err != nil {
				return err
			}
			var current unix.Stat_t
			if !bytes.Equal(data, []byte(intent.Original)) || unix.Fstat(int(source.Fd()), &current) != nil || current.Dev != original.Dev || current.Ino != original.Ino || current.Mode != original.Mode || current.Nlink != 1 || current.Uid != 0 || current.Gid != 0 || current.Size != original.Size {
				return ErrConflict
			}
			return ctx.Err()
		}
		if err := checkSource(); err != nil {
			return err
		}
		file, err := directory.OpenFile(".homenode-nsswitch.stage", os.O_CREATE|os.O_EXCL|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
		if err != nil {
			return err
		}
		defer func() { result = errors.Join(result, file.Close()) }()
		data := []byte(intent.Proposal.Contents)
		if n, err := file.Write(data); err != nil || n != len(data) {
			return errors.Join(io.ErrShortWrite, err)
		}
		if err := file.Sync(); err != nil {
			return err
		}
		if err := syncDirectory(directory, "."); err != nil {
			return err
		}
		var stat unix.Stat_t
		if unix.Fstat(int(file.Fd()), &stat) != nil || stat.Mode != unix.S_IFREG|0600 || stat.Nlink != 1 || stat.Uid != 0 || stat.Gid != 0 || stat.Size != int64(len(data)) {
			return ErrConflict
		}
		encoded, err := json.Marshal(intent)
		if err != nil {
			return err
		}
		stage = guestIdentityNameServiceStage{Version: 1, IntentSHA256: digest(encoded), SourceDevice: uint64(original.Dev), SourceInode: original.Ino, Device: uint64(stat.Dev), Inode: stat.Ino, Bytes: stat.Size}
		if err := guard(ctx); err != nil {
			return err
		}
		if err := checkSource(); err != nil {
			return err
		}
		checkStage := func() error {
			contents, err := io.ReadAll(io.NewSectionReader(file, 0, int64(len(data))+1))
			if err != nil {
				return err
			}
			var current unix.Stat_t
			opened, statErr := file.Stat()
			named, nameErr := directory.Lstat(".homenode-nsswitch.stage")
			if !bytes.Equal(contents, data) || unix.Fstat(int(file.Fd()), &current) != nil || current.Dev != stat.Dev || current.Ino != stat.Ino || current.Mode != stat.Mode || current.Nlink != 1 || current.Uid != 0 || current.Gid != 0 || current.Size != stat.Size || statErr != nil || nameErr != nil || !os.SameFile(opened, named) {
				return ErrConflict
			}
			return ctx.Err()
		}
		if err := checkStage(); err != nil {
			return err
		}
		encodedStage, err := json.Marshal(stage)
		if err != nil {
			return err
		}
		if err := e.commitImmutableGuestIntent(ctx, "guest-identity-nss-stage.json", "guest-identity-nss-stage", encodedStage); err != nil {
			return err
		}
		return checkStage()
	})
	if result != nil {
		stage = guestIdentityNameServiceStage{}
	}
	return stage, result
}
