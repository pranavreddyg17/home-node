package install

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
	"os"
	"runtime"
	"testing"
)

// Called only by the opt-in disposable Linux account fixture, after its legacy
// storage checks. No services are started and no TLS files are changed here.
func qualifyNativeGatewayAccounts(t *testing.T, engine *Engine, ctx context.Context) {
	t.Helper()
	if os.Getenv("HOMENODE_ACCOUNT_INSPECTION_INTEGRATION") != "1" || runtime.GOOS != "linux" || os.Geteuid() != 0 {
		t.Fatal("disposable Linux root fixture required")
	}
	plan, err := engine.PrepareGatewayAccount(ctx)
	if err != nil {
		t.Fatal("native gateway preparation", err)
	}
	identity, err := engine.ProvisionGatewayAccount(ctx)
	if err != nil || identity != plan.Identity {
		t.Fatal("native gateway provisioning", identity, err)
	}
	if replay, err := engine.ProvisionGatewayAccount(ctx); err != nil || replay != identity {
		t.Fatal("native gateway replay", replay, err)
	}
	inspected, err := InspectGatewayAccount(ctx)
	if err != nil || inspected != identity {
		t.Fatal("live gateway isolation", inspected, err)
	}
	output, err := accountCommand(ctx, "/usr/bin/homenode", "gateway-accounts-check")
	if err != nil {
		t.Fatal("installed gateway check CLI", err)
	}
	var result struct {
		Valid     bool           `json:"gatewayAccountValid"`
		Identity  GatewayAccount `json:"identity"`
		Activated bool           `json:"servicesActivated"`
	}
	if err := json.Unmarshal(output, &result); err != nil || !result.Valid || result.Activated || result.Identity != identity {
		t.Fatal("gateway check status", result, err)
	}
	if _, err := accountCommand(ctx, "/usr/sbin/usermod", "--append", "--groups", "homenode-runtime", "homenode-gateway"); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectGatewayAccount(ctx); err == nil {
		t.Fatal("gateway runtime membership accepted")
	}
	if _, err := engine.ProvisionGatewayAccount(ctx); err == nil {
		t.Fatal("unsafe gateway readiness replay accepted")
	}
	if _, err := accountCommand(ctx, "/usr/sbin/usermod", "--groups", "homenode-proxy", "homenode-gateway"); err != nil {
		t.Fatal(err)
	}
	if actual, err := InspectGatewayAccount(ctx); err != nil || actual != identity {
		t.Fatal("repaired gateway refused", actual, err)
	}
	if _, err := accountCommand(ctx, "/usr/sbin/usermod", "--append", "--groups", "homenode-proxy", "homenode-transfer"); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectGatewayAccount(ctx); err == nil {
		t.Fatal("transfer entered gateway bridge")
	}
	if _, err := accountCommand(ctx, "/usr/sbin/usermod", "--groups", "homenode-runtime", "homenode-transfer"); err != nil {
		t.Fatal(err)
	}
	if actual, err := InspectGatewayAccount(ctx); err != nil || actual != identity {
		t.Fatal("repaired bridge refused", actual, err)
	}
	uidPlan, err := engine.PlanInstalledGuestUIDProvisioning(ctx, supervisor.GuestUIDPool{First: 2000000000, Last: 2000000001})
	if err != nil || len(uidPlan.ServiceUIDs) != 4 {
		t.Fatal("gateway omitted from guest UID service exclusions", uidPlan, err)
	}
	found := false
	for _, uid := range uidPlan.ServiceUIDs {
		if uid == identity.UID {
			found = true
		}
	}
	if !found {
		t.Fatal("guest UID plan lacks verified gateway UID")
	}
	t.Log("native journaled gateway provisioning, installed check CLI, readiness replay and group isolation passed")
}

// Runs after native NSS/allocator restoration while owned accounts still exist.
func qualifyNativeGatewayRemoval(t *testing.T, engine *Engine, ctx context.Context) {
	t.Helper()
	if os.Getenv("HOMENODE_ACCOUNT_INSPECTION_INTEGRATION") != "1" || runtime.GOOS != "linux" || os.Geteuid() != 0 {
		t.Fatal("disposable Linux root fixture required")
	}
	var err error
	// This fixture has never activated services. It qualifies real account command
	// semantics only; its guard does not claim installed systemd exclusion.
	if _, err := engine.PrepareGatewayAccountRemoval(ctx); err != nil {
		t.Fatal("native removal intent", err)
	}
	lost := errors.New("fixture interrupted removal acknowledgement")
	engine.checkpoint = func(stage, name string) error {
		if stage == "gateway-account-removed" {
			return lost
		}
		return nil
	}
	engine.mu.Lock()
	err = engine.removeGatewayAccountLocked(ctx, nativeGatewayProvisioner{}, func(ctx context.Context) error { return ctx.Err() })
	engine.mu.Unlock()
	engine.checkpoint = nil
	if !errors.Is(err, lost) {
		t.Fatal("native removal interruption", err)
	}
	engine.mu.Lock()
	err = engine.removeGatewayAccountLocked(ctx, nativeGatewayProvisioner{}, func(ctx context.Context) error { return ctx.Err() })
	engine.mu.Unlock()
	if err != nil {
		t.Fatal("native removal recovery", err)
	}
	engine.mu.Lock()
	err = engine.removeGatewayAccountLocked(ctx, nativeGatewayProvisioner{}, func(ctx context.Context) error { return ctx.Err() })
	engine.mu.Unlock()
	if err != nil {
		t.Fatal("native removal readiness replay", err)
	}
	if _, err := InspectLocalAccounts(ctx); err != nil {
		t.Fatal("retained base account isolation", err)
	}
	t.Log("native journaled gateway account removal and interrupted acknowledgement recovery passed; service exclusion not exercised")

}
