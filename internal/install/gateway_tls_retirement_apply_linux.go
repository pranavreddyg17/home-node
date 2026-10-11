//go:build linux

package install

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
)

// Internal mutation consumer: caller holds the installation lock, retained inode
// descriptors and a live authority guard proving activation exclusion, gateway
// identity and manager dormancy. No identity is deleted or barrier released here.
func (e *Engine) applyGatewayTLSRetirementLocked(ctx context.Context, a *gatewayTLSAdmission, guard func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if a == nil || guard == nil {
		return ErrConflict
	}
	data, err := e.readGatewayTLSRetirementIntent()
	if err != nil {
		return err
	}
	intent, err := decodeGatewayTLSRetirementIntent(data, a.Plan)
	if err != nil {
		return err
	}
	observe := func() (int, error) {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		if err := guard(); err != nil {
			return 0, err
		}
		installed, err := e.load()
		if err != nil || installed.Phase != "installed" || installed.ID != intent.Plan.InstallationID {
			return 0, ErrConflict
		}
		if _, err := gatewayTLSInstallationSource(installed, intent); err != nil {
			return 0, err
		}
		current, err := e.readGatewayTLSRetirementIntent()
		if err != nil {
			return 0, err
		}
		expected, err := json.Marshal(intent)
		if err != nil || !bytes.Equal(current, expected) {
			return 0, ErrConflict
		}
		phase, err := e.observeGatewayTLSRetirementLocked(ctx, a, intent)
		if err != nil {
			return 0, err
		}
		if err := guard(); err != nil {
			return 0, err
		}
		return phase, ctx.Err()
	}
	if _, err := observe(); err != nil {
		return err
	}
	if intent.Ready {
		return nil
	}
	for intent.Completed < len(intent.Plan.Changes) {
		index := intent.Completed
		change := intent.Plan.Changes[index]
		phase, err := observe()
		if err != nil {
			return err
		}
		var retained *os.File
		for i, source := range a.Sources {
			if source.Path == change.Path {
				retained = a.files[i]
			}
		}
		if retained == nil {
			return ErrConflict
		}
		if phase == index {
			if change.BeforeGID != change.AfterGID {
				if change.BeforeMode != change.AfterMode {
					return ErrConflict
				}
				if err := retained.Chown(0, change.AfterGID); err != nil {
					return err
				}
			} else if change.BeforeMode != change.AfterMode {
				if err := retained.Chmod(os.FileMode(change.AfterMode)); err != nil {
					return err
				}
			}
		} else if phase != index+1 {
			return ErrConflict
		}
		// Sync even on a lost acknowledgement before advancing durable progress.
		if err := retained.Sync(); err != nil {
			return err
		}
		phase, err = observe()
		if err != nil || phase != index+1 {
			if err != nil {
				return err
			}
			return ErrConflict
		}
		if e.checkpoint != nil {
			if err := e.checkpoint("gateway-tls-retired", change.Path); err != nil {
				return err
			}
		}
		phase, err = observe()
		if err != nil || phase != index+1 {
			if err != nil {
				return err
			}
			return ErrConflict
		}
		next := intent
		next.Completed = index + 1
		encoded, err := json.Marshal(next)
		if err != nil {
			return err
		}
		if err := e.saveJournalBytes("gateway-tls-retirement", encoded); err != nil {
			return err
		}
		intent = next
	}
	if _, err := observe(); err != nil {
		return err
	}
	intent.Ready = true
	encoded, err := json.Marshal(intent)
	if err != nil {
		return err
	}
	if err := e.saveJournalBytes("gateway-tls-retirement", encoded); err != nil {
		return err
	}
	_, err = observe()
	return err
}
