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

type guestStorageConfigurationStageFile struct {
	Name         string `json:"name"`
	SourceDevice uint64 `json:"sourceDevice"`
	SourceInode  uint64 `json:"sourceInode"`
	Device       uint64 `json:"device"`
	Inode        uint64 `json:"inode"`
	Bytes        int64  `json:"bytes"`
	SHA256       string `json:"sha256"`
}
type guestStorageConfigurationStage struct {
	Version      int                                  `json:"version"`
	IntentSHA256 string                               `json:"intentSha256"`
	Files        []guestStorageConfigurationStageFile `json:"files"`
}

// Create both replacements while retaining the originals and immutable intent.
// Unrecorded partial files remain evidence; matching bytes never authorize their
// adoption. Publication and interrupted-stage recovery are separate operations.
func (e *Engine) stageGuestStorageConfiguration(ctx context.Context, current journal, plan GuestStorageProvisioningPlan, guard func(context.Context) error) (stage guestStorageConfigurationStage, result error) {
	result = e.withGuestStorageConfigurationSources(ctx, current, plan, guard, func(ctx context.Context, intent guestStorageConfigurationIntent, policy, environment *os.File, check func() error) (result error) {
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
		encoded, err := json.Marshal(intent)
		if err != nil {
			return err
		}
		stage = guestStorageConfigurationStage{Version: 1, IntentSHA256: digest(encoded)}
		var files []*os.File
		defer func() {
			for _, file := range files {
				result = errors.Join(result, file.Close())
			}
		}()
		sources := []*os.File{policy, environment}
		payloads := [][]byte{intent.Policy, intent.Environment}
		names := []string{".homenode-runtime-policy.stage", ".homenode-services-env.stage"}
		for i, name := range names {
			if err := check(); err != nil {
				return err
			}
			var source unix.Stat_t
			if unix.Fstat(int(sources[i].Fd()), &source) != nil {
				return ErrConflict
			}
			file, err := directory.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_RDWR|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
			if err != nil {
				return err
			}
			files = append(files, file)
			if n, err := file.Write(payloads[i]); err != nil || n != len(payloads[i]) {
				return errors.Join(io.ErrShortWrite, err)
			}
			if err := file.Sync(); err != nil {
				return err
			}
			var pending unix.Stat_t
			if unix.Fstat(int(file.Fd()), &pending) != nil || pending.Mode != unix.S_IFREG|0600 || pending.Uid != 0 || pending.Gid != 0 || pending.Nlink != 1 || pending.Size != int64(len(payloads[i])) || pending.Dev != source.Dev || pending.Ino == source.Ino {
				return ErrConflict
			}
			stage.Files = append(stage.Files, guestStorageConfigurationStageFile{Name: name, SourceDevice: uint64(source.Dev), SourceInode: source.Ino, Device: uint64(pending.Dev), Inode: pending.Ino, Bytes: pending.Size, SHA256: digest(payloads[i])})
		}
		if err := parent.Sync(); err != nil {
			return err
		}
		verify := func() error {
			if err := check(); err != nil {
				return err
			}
			opened, err := parent.Stat()
			named, nameErr := e.host.Lstat("etc/homenode")
			if err != nil || nameErr != nil || !os.SameFile(opened, named) {
				return ErrConflict
			}
			for i, file := range files {
				receipt := stage.Files[i]
				data, err := io.ReadAll(io.NewSectionReader(file, 0, receipt.Bytes+1))
				if err != nil {
					return err
				}
				var observed, named unix.Stat_t
				if unix.Fstat(int(file.Fd()), &observed) != nil || unix.Fstatat(int(parent.Fd()), receipt.Name, &named, unix.AT_SYMLINK_NOFOLLOW) != nil || uint64(observed.Dev) != receipt.Device || observed.Ino != receipt.Inode || observed.Mode != unix.S_IFREG|0600 || observed.Uid != 0 || observed.Gid != 0 || observed.Nlink != 1 || observed.Size != receipt.Bytes || named.Dev != observed.Dev || named.Ino != observed.Ino || !bytes.Equal(data, payloads[i]) {
					return ErrConflict
				}
			}
			return check()
		}
		if err := verify(); err != nil {
			return err
		}
		record, err := json.Marshal(stage)
		if err != nil {
			return err
		}
		if err := e.commitImmutableGuestIntent(ctx, "guest-storage-configuration-stage.json", "guest-storage-configuration-stage", record); err != nil {
			return err
		}
		return verify()
	})
	if result != nil {
		stage = guestStorageConfigurationStage{}
	}
	return stage, result
}
