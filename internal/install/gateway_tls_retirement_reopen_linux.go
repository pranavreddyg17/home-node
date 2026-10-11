//go:build linux

package install

import (
	"context"
	"errors"
	"os"
	"syscall"
)

// Reopening is read-only and takes an independently reconstructed source plan.
// The recorded device/inode/hash and admissible phase are checked before any
// descriptor is returned. Caller retains the lock and supplies live authority
// separately before mutation; an old journal alone cannot authorize retirement.
func (e *Engine) reopenGatewayTLSRetirementLocked(ctx context.Context, source GatewayTLSRetirementPlan) (result *gatewayTLSAdmission, resultErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := e.readGatewayTLSRetirementIntent()
	if err != nil {
		return nil, err
	}
	intent, err := decodeGatewayTLSRetirementIntent(data, source)
	if err != nil {
		return nil, err
	}
	installed, err := e.load()
	if err != nil || installed.Phase != "installed" || installed.ID != source.InstallationID || installed.Digest != source.InstallationDigest {
		return nil, ErrConflict
	}
	reconstructed, err := gatewayTLSRetirementPlan(installed, source.Identity, source.Key, source.Certificate)
	if err != nil {
		return nil, err
	}
	if _, err := decodeGatewayTLSRetirementIntent(data, reconstructed); err != nil {
		return nil, err
	}
	a := &gatewayTLSAdmission{Plan: source, Sources: intent.Sources}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, a.Close())
		}
	}()
	for _, item := range intent.Sources {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := e.parents(item.Path); err != nil {
			return nil, err
		}
		file, err := e.host.OpenFile(item.Path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		if err != nil {
			return nil, err
		}
		a.files = append(a.files, file)
	}
	if _, err := e.observeGatewayTLSRetirementLocked(ctx, a, intent); err != nil {
		return nil, err
	}
	// Refuse journal replacement while descriptors were being reopened.
	current, err := e.readGatewayTLSRetirementIntent()
	if err != nil {
		return nil, err
	}
	if string(current) != string(data) {
		return nil, ErrConflict
	}
	return a, nil
}
