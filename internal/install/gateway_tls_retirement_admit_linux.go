//go:build linux

package install

import (
	"context"
	"reflect"
)

// Installation admission for withRecoveryInstallationExclusionGuardedLocked.
// The retained immutable source limits the only tolerated drift to the planned
// TLS directory permissions. TLS contents, ACLs and inode identity are always
// checked by the phase observer. Live manager/identity checks remain mandatory
// in the enclosing guard; this function grants no authority on its own.
func (e *Engine) admitGatewayTLSInstallationLocked(ctx context.Context, current, immutable journal, a *gatewayTLSAdmission) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if a == nil {
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
	source, err := gatewayTLSInstallationSource(current, intent)
	if err != nil || !reflect.DeepEqual(source, immutable) {
		return ErrConflict
	}
	for _, item := range source.Items {
		if err := ctx.Err(); err != nil {
			return err
		}
		if item.Path == "etc/homenode/tls" {
			continue
		}
		if err := e.matches(item); err != nil {
			return err
		}
	}
	if _, err := e.observeGatewayTLSRetirementLocked(ctx, a, intent); err != nil {
		return err
	}
	return ctx.Err()
}
