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
)

// Load the coordinated stage receipt against an independently qualified plan.
// This does not adopt any host inode; publication must requalify both paths.
func (e *Engine) loadGuestStorageConfigurationStage(ctx context.Context, current journal, plan GuestStorageProvisioningPlan) (stage guestStorageConfigurationStage, result error) {
	result = e.withGuestStorageConfigurationIntent(ctx, current, plan, func(intent guestStorageConfigurationIntent, checkIntent func() error) (result error) {
		const name = "guest-storage-configuration-stage.json"
		file, err := e.journalRoot.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		if err != nil {
			return err
		}
		defer func() { result = errors.Join(result, file.Close()) }()
		before, err := file.Stat()
		if err != nil || !accountJournalFileAdmitted(before, e.owner, 8192) || !e.accountJournalPathUnchanged(name, before, 8192) {
			return ErrConflict
		}
		data, err := io.ReadAll(io.LimitReader(file, 8193))
		if err != nil {
			return err
		}
		if len(data) > 8192 || json.Unmarshal(data, &stage) != nil {
			return ErrConflict
		}
		canonical, err := json.Marshal(stage)
		if err != nil {
			return err
		}
		encodedIntent, err := json.Marshal(intent)
		if err != nil {
			return err
		}
		if !bytes.Equal(data, canonical) || stage.Version != 1 || len(stage.Files) != 2 || stage.IntentSHA256 != digest(encodedIntent) {
			return ErrConflict
		}
		names := []string{".homenode-runtime-policy.stage", ".homenode-services-env.stage"}
		payloads := [][]byte{intent.Policy, intent.Environment}
		for i, receipt := range stage.Files {
			if receipt.Name != names[i] || receipt.Inode == 0 || receipt.SourceInode == 0 || receipt.Device != receipt.SourceDevice || receipt.Inode == receipt.SourceInode || receipt.Bytes != int64(len(payloads[i])) || receipt.SHA256 != digest(payloads[i]) {
				return ErrConflict
			}
		}
		first, second := stage.Files[0], stage.Files[1]
		if first.Device != second.Device || first.Inode == second.Inode || first.SourceInode == second.SourceInode || first.Inode == second.SourceInode || second.Inode == first.SourceInode {
			return ErrConflict
		}
		if err := checkIntent(); err != nil {
			return err
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return err
		}
		currentBytes, err := io.ReadAll(io.LimitReader(file, 8193))
		if err != nil {
			return err
		}
		after, err := file.Stat()
		if err != nil || !accountJournalFileAdmitted(after, e.owner, 8192) || !os.SameFile(before, after) || after.Size() != int64(len(data)) || !bytes.Equal(currentBytes, data) || !e.accountJournalPathUnchanged(name, before, 8192) {
			return ErrConflict
		}
		return ctx.Err()
	})
	if result != nil {
		stage = guestStorageConfigurationStage{}
	}
	return stage, result
}
