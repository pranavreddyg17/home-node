package install

import (
	"context"
	"encoding/json"
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
	t.Log("native journaled gateway provisioning, installed check CLI, readiness replay and group isolation passed")
}
