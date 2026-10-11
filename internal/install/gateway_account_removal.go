package install

import (
	"context"
	"os"
	"runtime"
	"time"
)

type GatewayAccountRemovalPlan struct {
	OwnerID  string           `json:"ownerId"`
	Identity GatewayAccount   `json:"identity"`
	Commands []AccountCommand `json:"commands"`
	Pending  []string         `json:"pending"`
}

// PlanGatewayAccountRemoval is read-only. It does not stop services, remove
// identities or erase any application/backup data. Execution requires retained
// activation exclusion, native dormancy and its own durable removal journal.
func (e *Engine) PlanGatewayAccountRemoval(ctx context.Context) (GatewayAccountRemovalPlan, error) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 || e.host.Name() != "/" {
		return GatewayAccountRemovalPlan{}, ErrAccounts
	}
	deadline, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return e.planGatewayAccountRemoval(deadline, nativeGatewayProvisioner{})
}
func (e *Engine) planGatewayAccountRemoval(ctx context.Context, b gatewayProvisionBackend) (GatewayAccountRemovalPlan, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.planGatewayAccountRemovalLocked(ctx, b)
}
func (e *Engine) planGatewayAccountRemovalLocked(ctx context.Context, b gatewayProvisionBackend) (GatewayAccountRemovalPlan, error) {
	var empty GatewayAccountRemovalPlan
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	base, err := e.loadAccountJournal()
	if err != nil || !base.Ready {
		return empty, ErrConflict
	}
	identity, err := e.observeGatewayAccountWith(ctx, base, b)
	if err != nil {
		return empty, err
	}
	if identity == nil {
		return empty, ErrConflict
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	return GatewayAccountRemovalPlan{OwnerID: base.OwnerID, Identity: *identity, Commands: []AccountCommand{
		{Program: "/usr/bin/gpasswd", Arguments: []string{"--delete", "homenode", "homenode-proxy"}},
		{Program: "/usr/sbin/userdel", Arguments: []string{"homenode-gateway"}},
		{Program: "/usr/sbin/groupdel", Arguments: []string{"homenode-proxy"}},
		{Program: "/usr/sbin/groupdel", Arguments: []string{"homenode-gateway"}},
	}, Pending: []string{"retain installed activation exclusion and verify gateway/controller dormancy", "journal removal intent and reconcile every command acknowledgement", "remove only owned service configuration while retaining workload and backup data", "qualify native removal and interrupted recovery before executing this plan"}}, nil
}
