//go:build linux

package install

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// This temporary root filesystem fixture proves installed record verification,
// not live service identity, TLS trust, guest launch or confinement.
func TestRootPreparedGatewayRequiresOwnedJournalAndExactUnits(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root-only temporary Linux fixture")
	}
	for _, scenario := range []string{"valid", "missing gateway journal", "unready gateway journal"} {
		t.Run(scenario, func(t *testing.T) {
			identity := GatewayAccount{UID: 803, GID: 803, ProxyGID: 804}
			e, c, _, journal, source, now := imagePlacementFixtureWithIdentities(t, nil, &identity)
			defer e.Close()
			readyAccountIntent(t, e, c.Accounts, true)
			base, err := e.loadAccountJournal()
			if err != nil {
				t.Fatal(err)
			}
			gateway := gatewayAccountJournal{Version: 1, Completed: 4, Ready: true, Plan: GatewayAccountPlan{OwnerID: base.OwnerID, Identity: identity, Commands: gatewayCreationCommands(base.OwnerID, identity)}}
			if scenario == "unready gateway journal" {
				gateway.Ready = false
			}
			data, err := json.Marshal(gateway)
			if err != nil {
				t.Fatal(err)
			}
			if err := e.saveJournalBytes("gateway-accounts", data); err != nil {
				t.Fatal(err)
			}
			if err := e.placeImages(context.Background(), source, c.Publisher, c.MinimumCatalogVersion, now); err != nil {
				t.Fatal(err)
			}
			if scenario == "missing gateway journal" {
				if err := os.Remove(filepath.Join(journal, "gateway-accounts.json")); err != nil {
					t.Fatal(err)
				}
			}
			result, err := e.checkPrepared(context.Background(), now)
			if scenario == "valid" {
				if err != nil || result.Gateway == nil || *result.Gateway != identity || !result.ArtifactsVerified || result.AccountsVerified || result.HTTPSIdentityVerified {
					t.Fatal("gateway prepared scope", result, err)
				}
			} else if err == nil {
				t.Fatal("unowned gateway configuration admitted", scenario)
			}
		})
	}
}
