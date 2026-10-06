package install

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/pranavreddyg17/home-node/internal/backup"
	"github.com/pranavreddyg17/home-node/internal/catalog"
)

// stageRecoveryCopies keeps installer exclusion through immutable intent,
// scoped source qualification, copying and retry reconciliation. Files remain
// disconnected under installer ownership in the private journal directory.
func (e *Engine) stageRecoveryCopies(ctx context.Context, prepared *backup.PreparedRecoveryLease, source backup.Manifest, c Configuration, now time.Time) (RecoveryConfigurationPreview, error) {
	// One deadline bounds all repeated qualification/copy/reconciliation work,
	// rather than granting a fresh full timeout to each phase.
	ctx, cancel := context.WithTimeout(ctx, 2*time.Hour)
	defer cancel()
	e.mu.Lock()
	defer e.mu.Unlock()
	preview, err := e.prepareRecoveryIntentLocked(ctx, prepared, source, c, now)
	if err != nil {
		return RecoveryConfigurationPreview{}, err
	}
	if e.checkpoint != nil {
		if err = e.checkpoint("recovery-intent", preview.Recovery.SnapshotID); err != nil {
			return RecoveryConfigurationPreview{}, err
		}
	}
	manifest, err := catalog.Verify(c.Catalog, map[string]ed25519.PublicKey{catalog.KeyID(c.Publisher): c.Publisher}, c.MinimumCatalogVersion, now)
	if err != nil {
		return RecoveryConfigurationPreview{}, err
	}
	approved := make(map[string]string, 2)
	for _, workload := range []string{"files", "ai"} {
		image, err := manifest.Image(workload)
		if err != nil {
			return RecoveryConfigurationPreview{}, err
		}
		approved[workload] = image.SHA256
	}
	expected, _ := json.Marshal(preview.Recovery)
	err = prepared.WithFiles(ctx, c.Maintenance.UID, source, backup.RestorePolicy{MinimumCatalogVersion: c.MinimumCatalogVersion, ApprovedImages: approved}, func(ctx context.Context, inventory backup.PreparedRecoveryInventory, files []backup.PreparedRecoveryFile) error {
		current, _ := json.Marshal(inventory)
		if !bytes.Equal(expected, current) {
			return ErrConflict
		}
		for _, file := range files {
			stage := ".recovery-management.copy"
			if file.Name != "management.db" {
				stage = ".recovery-" + strings.TrimSuffix(file.Name, ".raw") + ".copy"
			}
			err := verifyRecoveryCopy(ctx, e.journalRoot, stage, file, e.owner)
			if errors.Is(err, os.ErrNotExist) {
				if err = copyRecoveryFile(ctx, e.journalRoot, stage, file, e.owner); err != nil {
					return err
				}
				err = verifyRecoveryCopy(ctx, e.journalRoot, stage, file, e.owner)
			}
			if err != nil {
				return err
			}
			if e.checkpoint != nil {
				if err = e.checkpoint("recovery-copy", stage); err != nil {
					return err
				}
			}
		}
		return ctx.Err()
	})
	if err != nil {
		return RecoveryConfigurationPreview{}, err
	}
	// Record completion only after borrowed descriptors have closed successfully.
	installed, err := e.load()
	if err != nil {
		return RecoveryConfigurationPreview{}, err
	}
	intent, err := json.Marshal(recoveryIntent{Version: 1, ConfigurationID: installed.ID, ConfigurationDigest: installed.Digest, Recovery: preview.Recovery})
	if err != nil {
		return RecoveryConfigurationPreview{}, err
	}
	receipt, err := json.Marshal(recoveryStagedReceipt{Version: 1, IntentSHA256: digest(intent), Files: len(preview.Recovery.Disks) + 1})
	if err != nil {
		return RecoveryConfigurationPreview{}, err
	}
	if err = e.commitRecoveryRecord(ctx, "recovery-staged.json", receipt); err != nil {
		return RecoveryConfigurationPreview{}, err
	}
	if err = e.reconcileRecoveryStaging(ctx, recoveryIntent{Version: 1, ConfigurationID: installed.ID, ConfigurationDigest: installed.Digest, Recovery: preview.Recovery}); err != nil {
		return RecoveryConfigurationPreview{}, err
	}
	return preview, nil
}

// Receipt identifies completed disconnected staging, not installed ownership,
// runtime admission, application health or client enrollment.
type recoveryStagedReceipt struct {
	Version      int    `json:"version"`
	IntentSHA256 string `json:"intentSha256"`
	Files        int    `json:"files"`
}
