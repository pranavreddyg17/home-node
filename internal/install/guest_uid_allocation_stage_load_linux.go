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

// Load committed staging for a restart using independently selected intent.
// This does not adopt any host inode; publication must requalify both paths.
func (e *Engine) loadGuestUIDAllocationStage(ctx context.Context, intent guestUIDAllocationIntent) (stage guestUIDAllocationStage, result error) {
	result = e.withGuestUIDAllocationIntent(ctx, intent, func(ctx context.Context, _ func() error) (result error) {
		const name = "guest-uid-allocation-stage.json"
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
		encodedIntent, err := encodeGuestUIDAllocationIntent(ctx, intent)
		if err != nil {
			return err
		}
		if !bytes.Equal(data, canonical) || stage.Version != 1 || stage.Inode == 0 || stage.SourceInode == 0 || stage.Device != stage.SourceDevice || stage.Inode == stage.SourceInode || stage.Bytes != int64(len(intent.Proposal.Contents)) || stage.IntentSHA256 != digest(encodedIntent) {
			return ErrConflict
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
		stage = guestUIDAllocationStage{}
	}
	return stage, result
}
