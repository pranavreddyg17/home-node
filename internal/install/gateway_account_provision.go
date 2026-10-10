package install

import (
	"context"
	"encoding/json"
	"os"
	"runtime"
	"strconv"
	"time"
)

type gatewayProvisionBackend interface {
	Snapshot(context.Context) (accountSnapshot, error)
	Lookup(context.Context, string, string) (bool, error)
	Execute(context.Context, AccountCommand) error
	VerifyGateway(context.Context) (GatewayAccount, error)
}
type nativeGatewayProvisioner struct{ nativeAccountProvisioner }

func (nativeGatewayProvisioner) VerifyGateway(ctx context.Context) (GatewayAccount, error) {
	snapshot, err := (nativeAccountProvisioner{}).Snapshot(ctx)
	defer clear(snapshot.shadow)
	if err != nil {
		return GatewayAccount{}, err
	}
	accounts, identity, err := validateGatewayAccounts(snapshot.passwd, snapshot.groups, snapshot.shadow)
	if err != nil {
		return GatewayAccount{}, err
	}
	if err := resolvedAccounts(ctx, accounts, &identity); err != nil {
		return GatewayAccount{}, err
	}
	return identity, nil
}

// ProvisionGatewayAccount runs only a previously committed creation intent.
// Failures retain that intent; an uncertain command result is reconciled against
// the exact owned account attributes before any command is retried.
func (e *Engine) ProvisionGatewayAccount(ctx context.Context) (GatewayAccount, error) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 || e.host.Name() != "/" {
		return GatewayAccount{}, ErrAccounts
	}
	deadline, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	return e.provisionGatewayAccount(deadline, nativeGatewayProvisioner{})
}
func (e *Engine) provisionGatewayAccount(ctx context.Context, b gatewayProvisionBackend) (GatewayAccount, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	var empty GatewayAccount
	base, err := e.loadAccountJournal()
	if err != nil || !base.Ready {
		return empty, ErrConflict
	}
	j, err := e.loadGatewayAccountJournal(base)
	if err != nil {
		return empty, err
	}
	for index, command := range j.Plan.Commands {
		if err = ctx.Err(); err != nil {
			return empty, err
		}
		snapshot, err := b.Snapshot(ctx)
		defer clear(snapshot.shadow)
		if err != nil {
			return empty, err
		}
		for step := 0; step < 5; step++ {
			ready, err := gatewayBaseStepMatches(snapshot, base, step)
			if err != nil || !ready {
				return empty, ErrConflict
			}
		}
		present, err := gatewayAccountStepMatches(snapshot, j.Plan, index)
		if err != nil {
			return empty, err
		}
		if !present {
			if index < j.Completed || j.Ready {
				return empty, ErrConflict
			}
			database, id, name := "group", strconv.Itoa(j.Plan.Identity.GID), "homenode-gateway"
			if index == 1 {
				id = strconv.Itoa(j.Plan.Identity.ProxyGID)
				name = "homenode-proxy"
			}
			if index == 2 {
				database = "passwd"
				id = strconv.FormatUint(uint64(j.Plan.Identity.UID), 10)
			}
			keys := []string{name, id}
			if index == 3 {
				keys = nil
			} // Existing owned controller gains only the proxy membership.
			for _, key := range keys {
				exists, err := b.Lookup(ctx, database, key)
				if err != nil {
					return empty, err
				}
				if exists {
					return empty, ErrConflict
				}
			}
			if err = ctx.Err(); err != nil {
				return empty, err
			}
			if err = b.Execute(ctx, command); err != nil {
				return empty, err
			}
			snapshot, err = b.Snapshot(ctx)
			defer clear(snapshot.shadow)
			if err != nil {
				return empty, err
			}
			present, err = gatewayAccountStepMatches(snapshot, j.Plan, index)
			if err != nil || !present {
				return empty, ErrConflict
			}
			if e.checkpoint != nil {
				if err = e.checkpoint("gateway-account-created", "homenode-gateway"); err != nil {
					return empty, err
				}
			}
		}
		if index >= j.Completed {
			j.Completed = index + 1
			data, err := json.Marshal(j)
			if err != nil {
				return empty, err
			}
			if err = e.saveJournalBytes("gateway-accounts", data); err != nil {
				return empty, err
			}
		}
	}
	actual, err := b.VerifyGateway(ctx)
	if err != nil {
		return empty, err
	}
	if actual != j.Plan.Identity {
		return empty, ErrConflict
	}
	if err = ctx.Err(); err != nil {
		return empty, err
	}
	if !j.Ready {
		j.Ready = true
		data, err := json.Marshal(j)
		if err != nil {
			return empty, err
		}
		if err = e.saveJournalBytes("gateway-accounts", data); err != nil {
			return empty, err
		}
	}
	return actual, nil
}
