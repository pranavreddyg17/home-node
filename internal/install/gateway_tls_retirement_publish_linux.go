//go:build linux

package install

import (
	"bytes"
	"context"
	"reflect"
)

// Caller holds the lock and live exclusion/identity/dormancy guard. Publication
// changes only the permission provenance already verified on retained TLS
// inodes. Ready replay also admits the exact previously published successor.
func (e *Engine) publishGatewayTLSRetirementLocked(ctx context.Context, a *gatewayTLSAdmission, guard func() error) error {
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
	if err != nil || !intent.Ready {
		return ErrConflict
	}
	current, err := e.load()
	if err != nil {
		return err
	}
	source, err := gatewayTLSInstallationSource(current, intent)
	if err != nil {
		return err
	}
	next, err := gatewayTLSRetiredInstallation(source, intent)
	if err != nil {
		return err
	}
	verify := func(expected journal) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := guard(); err != nil {
			return err
		}
		actual, err := e.load()
		if err != nil || !reflect.DeepEqual(actual, expected) {
			return ErrConflict
		}
		present, err := e.readGatewayTLSRetirementIntent()
		if err != nil || !bytes.Equal(present, data) {
			return ErrConflict
		}
		if phase, err := e.observeGatewayTLSRetirementLocked(ctx, a, intent); err != nil || phase != 5 {
			return ErrConflict
		}
		for _, item := range next.Items {
			if err := e.matches(item); err != nil {
				return err
			}
		}
		if err := guard(); err != nil {
			return err
		}
		return ctx.Err()
	}
	if err := verify(current); err != nil {
		return err
	}
	if !reflect.DeepEqual(current, next) {
		if err := e.save(next); err != nil {
			return err
		}
		if e.checkpoint != nil {
			if err := e.checkpoint("gateway-tls-installation-published", "etc/homenode/tls"); err != nil {
				return err
			}
		}
	}
	return verify(next)
}
