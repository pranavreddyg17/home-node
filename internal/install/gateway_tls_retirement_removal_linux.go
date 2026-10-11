//go:build linux

package install

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
)

// Retain the exact already-published root-only TLS successor before numeric
// identities are deleted. Journal flags alone are not proof of safe ownership.
func (e *Engine) retainRetiredGatewayTLSLocked(ctx context.Context, installed journal, identity GatewayAccount) (*gatewayTLSAdmission, func() error, error) {
	data, err := e.readGatewayTLSRetirementIntent()
	if err != nil {
		return nil, nil, err
	}
	var candidate gatewayTLSRetirementIntent
	if json.Unmarshal(data, &candidate) != nil || candidate.Plan.Identity != identity {
		return nil, nil, ErrConflict
	}
	intent, err := decodeGatewayTLSRetirementIntent(data, candidate.Plan)
	if err != nil || !intent.Ready {
		return nil, nil, ErrConflict
	}
	source, err := gatewayTLSSourceInstallation(installed, intent)
	if err != nil {
		return nil, nil, err
	}
	expected, err := gatewayTLSRetiredInstallation(source, intent)
	if err != nil || !reflect.DeepEqual(expected, installed) {
		return nil, nil, ErrConflict
	}
	a, err := e.reopenGatewayTLSRetirementLocked(ctx, intent.Plan)
	if err != nil {
		return nil, nil, err
	}
	verify := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		current, err := e.load()
		if err != nil || !reflect.DeepEqual(current, installed) {
			return ErrConflict
		}
		actual, err := e.readGatewayTLSRetirementIntent()
		if err != nil || !bytes.Equal(actual, data) {
			return ErrConflict
		}
		phase, err := e.observeGatewayTLSRetirementLocked(ctx, a, intent)
		if err != nil {
			return err
		}
		if phase != 5 {
			return ErrConflict
		}
		return ctx.Err()
	}
	if err := verify(); err != nil {
		a.Close()
		return nil, nil, err
	}
	return a, verify, nil
}
