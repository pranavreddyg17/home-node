package install

import (
	"context"
	"encoding/json"
	"strconv"
)

// removeGatewayAccountLocked is the teardown consumer, not a public mutation API.
// The installation exclusion owner must retain its lock and supply a guard that
// rechecks the marker, owned configuration, service dormancy and empty guests.
func (e *Engine) removeGatewayAccountLocked(ctx context.Context, b gatewayProvisionBackend, guard func(context.Context) error) error {
	if guard == nil {
		return ErrPlan
	}
	base, err := e.loadAccountJournal()
	if err != nil {
		return err
	}
	intent, err := e.loadGatewayRemovalIntent(base)
	if err != nil {
		return err
	}
	observe := func() (int, error) {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		if err := guard(ctx); err != nil {
			return 0, err
		}
		snapshot, err := b.Snapshot(ctx)
		defer clear(snapshot.shadow)
		if err != nil {
			return 0, err
		}
		for step := 0; step < 5; step++ {
			present, err := gatewayBaseStepMatches(snapshot, base, step)
			if err != nil || !present {
				return 0, ErrConflict
			}
		}
		phase, err := gatewayRemovalState(snapshot, intent.Plan)
		if err != nil {
			return 0, err
		}
		if phase < intent.Completed {
			return 0, ErrConflict
		}
		// Absence must also hold in NSS, not only in the local file snapshot.
		absent := []struct{ db, key string }{}
		if phase >= 2 {
			absent = append(absent, struct{ db, key string }{"passwd", "homenode-gateway"}, struct{ db, key string }{"passwd", strconv.FormatUint(uint64(intent.Plan.Identity.UID), 10)})
		}
		if phase >= 3 {
			absent = append(absent, struct{ db, key string }{"group", "homenode-proxy"}, struct{ db, key string }{"group", strconv.Itoa(intent.Plan.Identity.ProxyGID)})
		}
		privatePresent := false
		groups, err := accountLines(snapshot.groups, 4)
		if err != nil {
			return 0, err
		}
		for _, row := range groups {
			if row[0] == "homenode-gateway" {
				privatePresent = true
			}
		}
		// userdel may remove the private group before the proxy group. Verify
		// that absence immediately rather than deleting the proxy first.
		if !privatePresent {
			absent = append(absent, struct{ db, key string }{"group", "homenode-gateway"}, struct{ db, key string }{"group", strconv.Itoa(intent.Plan.Identity.GID)})
		}
		for _, key := range absent {
			exists, err := b.Lookup(ctx, key.db, key.key)
			if err != nil {
				return 0, err
			}
			if exists {
				return 0, ErrConflict
			}
		}
		if err := guard(ctx); err != nil {
			return 0, err
		}
		return phase, nil
	}
	persist := func() error {
		data, err := json.Marshal(intent)
		if err != nil {
			return err
		}
		return e.saveJournalBytes("gateway-account-removal", data)
	}
	for index, command := range intent.Plan.Commands {
		phase, err := observe()
		if err != nil {
			return err
		}
		if phase <= index {
			if intent.Ready || index < intent.Completed {
				return ErrConflict
			}
			if err := b.Execute(ctx, command); err != nil {
				return err
			}
			phase, err = observe()
			if err != nil {
				return err
			}
			if phase <= index {
				return ErrConflict
			}
			if e.checkpoint != nil {
				if err := e.checkpoint("gateway-account-removed", "homenode-gateway"); err != nil {
					return err
				}
			}
		}
		if index >= intent.Completed {
			intent.Completed = index + 1
			if err := persist(); err != nil {
				return err
			}
		}
	}
	phase, err := observe()
	if err != nil {
		return err
	}
	if phase != 4 {
		return ErrConflict
	}
	if !intent.Ready {
		intent.Ready = true
		return persist()
	}
	return nil
}
