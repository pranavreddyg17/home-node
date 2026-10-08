package install

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"syscall"
	"time"

	"github.com/pranavreddyg17/home-node/internal/backup"
)

type recoveryIntent struct {
	Version             int                              `json:"version"`
	ConfigurationID     string                           `json:"configurationId"`
	ConfigurationDigest string                           `json:"configurationDigest"`
	Recovery            backup.PreparedRecoveryInventory `json:"recovery"`
}

// prepareRecoveryIntent commits immutable disconnected recovery intent before
// later data effects. It requires an exact installed replacement configuration;
// production orchestration must independently observe accounts and capacity.
// It neither transfers data nor starts a controller.
func (e *Engine) prepareRecoveryIntent(ctx context.Context, prepared *backup.PreparedRecoveryLease, source backup.Manifest, c Configuration, now time.Time) (RecoveryConfigurationPreview, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.prepareRecoveryIntentLocked(ctx, prepared, source, c, now)
}

func (e *Engine) prepareRecoveryIntentLocked(ctx context.Context, prepared *backup.PreparedRecoveryLease, source backup.Manifest, c Configuration, now time.Time) (RecoveryConfigurationPreview, error) {
	if err := ctx.Err(); err != nil {
		return RecoveryConfigurationPreview{}, err
	}
	installed, err := e.load()
	if err != nil || installed.Phase != "installed" {
		return RecoveryConfigurationPreview{}, errors.Join(ErrConflict, err)
	}
	configuration, err := ConfigurationPlan(c, now)
	if err != nil {
		return RecoveryConfigurationPreview{}, err
	}
	_, expected, err := planRecords(configuration.Plan, e.owner)
	if err != nil || expected != installed.Digest {
		return RecoveryConfigurationPreview{}, ErrConflict
	}
	for _, record := range installed.Items {
		if err = e.matches(record); err != nil {
			return RecoveryConfigurationPreview{}, ErrConflict
		}
	}
	preview, err := RecoveryConfigurationPlan(ctx, prepared, source, c, now)
	if err != nil {
		return RecoveryConfigurationPreview{}, err
	}
	data, err := json.Marshal(recoveryIntent{Version: 1, ConfigurationID: installed.ID, ConfigurationDigest: installed.Digest, Recovery: preview.Recovery})
	if err != nil {
		return RecoveryConfigurationPreview{}, err
	}
	if err = e.commitRecoveryIntent(ctx, data); err != nil {
		return RecoveryConfigurationPreview{}, err
	}
	return preview, nil
}

// Exact canonical bytes are the retry proof. Existing ambiguous, truncated or
// foreign intent is preserved and refused rather than parsed/adopted.
func (e *Engine) commitRecoveryIntent(ctx context.Context, data []byte) error {
	return e.commitRecoveryRecord(ctx, "recovery.json", data)
}

func (e *Engine) commitRecoveryRecord(ctx context.Context, name string, data []byte) (result error) {
	if name != "recovery.json" && name != "recovery-staged.json" {
		return ErrPlan
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(data) == 0 || len(data) > 8192 {
		return ErrPlan
	}
	file, err := e.journalRoot.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY|syscall.O_NOFOLLOW, 0600)
	if errors.Is(err, os.ErrExist) {
		file, err = e.journalRoot.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		if err != nil {
			return err
		}
		defer func() { result = errors.Join(result, file.Close()) }()
		info, err := file.Stat()
		if err != nil || !accountJournalFileAdmitted(info, e.owner, 8192) || info.Size() != int64(len(data)) {
			return ErrConflict
		}
		actual, err := io.ReadAll(io.LimitReader(file, 8193))
		if err != nil || !bytes.Equal(actual, data) {
			return ErrConflict
		}
		current, err := e.journalRoot.Lstat(name)
		if err != nil || !os.SameFile(info, current) || !accountJournalFileAdmitted(current, e.owner, 8192) || current.Size() != int64(len(data)) {
			return ErrConflict
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		// Exact bytes can survive in cache after an interrupted initial write.
		// Complete both durability boundaries before accepting the retry.
		if err = file.Sync(); err != nil {
			return err
		}
		return syncDirectory(e.journalRoot, ".")
	}
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, file.Close()) }()
	// Preserve uncertain writes. They block handoff until explicit repair.
	if n, err := file.Write(data); err != nil || n != len(data) {
		return errors.Join(io.ErrShortWrite, err)
	}
	final, err := file.Stat()
	if err != nil || !accountJournalFileAdmitted(final, e.owner, 8192) || final.Size() != int64(len(data)) || !e.accountJournalPathUnchanged(name, final, 8192) {
		return ErrConflict
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = syncDirectory(e.journalRoot, "."); err != nil {
		return err
	}
	return ctx.Err()
}
