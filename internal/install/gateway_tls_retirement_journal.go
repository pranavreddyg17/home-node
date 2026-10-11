package install

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"syscall"
)

// Caller holds the installation lock. Source admission, named inode retention,
// owned gateway identity and activation exclusion belong to the required guard.
// This commits initial intent only and does not change TLS permissions.
func (e *Engine) prepareGatewayTLSRetirementIntentLocked(ctx context.Context, intent gatewayTLSRetirementIntent, guard func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if guard == nil || intent.Completed != 0 || intent.Ready {
		return ErrConflict
	}
	data, err := json.Marshal(intent)
	if err != nil {
		return err
	}
	if _, err = decodeGatewayTLSRetirementIntent(data, intent.Plan); err != nil {
		return err
	}
	if err = guard(); err != nil {
		return err
	}
	installed, err := e.load()
	if err != nil || installed.Phase != "installed" || installed.ID != intent.Plan.InstallationID || installed.Digest != intent.Plan.InstallationDigest {
		return ErrConflict
	}
	// Reconstruct permissions from the immutable installation source, including
	// rejection of other retained references to the retiring numeric identities.
	plan, err := gatewayTLSRetirementPlan(installed, intent.Plan.Identity, intent.Plan.Key, intent.Plan.Certificate)
	if err != nil {
		return err
	}
	if _, err = decodeGatewayTLSRetirementIntent(data, plan); err != nil {
		return err
	}
	existing, err := e.readGatewayTLSRetirementIntent()
	if err == nil {
		if !bytes.Equal(existing, data) {
			return ErrConflict
		}
		if err = guard(); err != nil {
			return err
		}
		return ctx.Err()
	}
	if !os.IsNotExist(err) {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = guard(); err != nil {
		return err
	}
	if err = e.saveJournalBytes("gateway-tls-retirement", data); err != nil {
		return err
	}
	if err = guard(); err != nil {
		return err
	}
	return ctx.Err()
}

func (e *Engine) readGatewayTLSRetirementIntent() ([]byte, error) {
	const name = "gateway-tls-retirement.json"
	file, err := e.journalRoot.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !accountJournalFileAdmitted(info, e.owner, gatewayTLSRetirementJournalLimit) {
		return nil, ErrConflict
	}
	data, err := io.ReadAll(io.LimitReader(file, gatewayTLSRetirementJournalLimit+1))
	if err != nil || len(data) > gatewayTLSRetirementJournalLimit || !e.accountJournalPathUnchanged(name, info, gatewayTLSRetirementJournalLimit) {
		return nil, ErrConflict
	}
	return data, nil
}
