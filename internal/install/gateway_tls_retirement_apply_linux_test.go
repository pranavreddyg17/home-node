//go:build linux

package install

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"
)

func TestRootGatewayTLSRetirementResumesEveryPermissionAcknowledgement(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root-only temporary filesystem fixture")
	}
	for trial := 1; trial <= 10; trial++ {
		stop := (trial-1)%5 + 1
		loseGuard := trial > 5
		t.Run(strconv.Itoa(trial), func(t *testing.T) {
			host, jr := roots(t)
			e := openEngine(t, host, jr)
			defer e.Close()
			c, _, _, now := configurationFixture(t)
			identity := GatewayAccount{UID: 803, GID: 803, ProxyGID: 804}
			c.Gateway = &identity
			preview, err := ConfigurationPlan(c, now)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			if err := e.Apply(ctx, preview.Plan); err != nil {
				t.Fatal(err)
			}
			key := "etc/homenode/tls/server.key"
			cert := "etc/homenode/tls/server.crt"
			if err := e.host.WriteFile(key, []byte("retained key fixture"), 0640); err != nil {
				t.Fatal(err)
			}
			if err := e.host.Chown(key, 0, identity.GID); err != nil {
				t.Fatal(err)
			}
			if err := e.host.WriteFile(cert, []byte("retained certificate fixture"), 0644); err != nil {
				t.Fatal(err)
			}
			a, err := e.admitGatewayTLSRetirementLocked(ctx, identity)
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			initial := gatewayTLSRetirementIntent{Version: 1, Plan: a.Plan, Sources: a.Sources}
			if err := e.prepareGatewayTLSRetirementIntentLocked(ctx, initial, func() error { return e.verifyGatewayTLSAdmissionLocked(ctx, a) }); err != nil {
				t.Fatal(err)
			}
			// This fixture proves permission/journal behavior only. Production must supply
			// the separately verified live manager, identity and activation barrier guard.
			interrupted := errors.New("lost TLS permission acknowledgement or authority")
			authority := true
			guard := func() error {
				if !authority {
					return interrupted
				}
				return nil
			}
			count := 0
			e.checkpoint = func(stage, name string) error {
				if stage == "gateway-tls-retired" {
					count++
					if count == stop {
						if loseGuard {
							authority = false
							return nil
						}
						return interrupted
					}
				}
				return nil
			}
			if err := e.applyGatewayTLSRetirementLocked(ctx, a, guard); !errors.Is(err, interrupted) {
				t.Fatal(err)
			}
			data, err := e.readGatewayTLSRetirementIntent()
			if err != nil {
				t.Fatal(err)
			}
			partial, err := decodeGatewayTLSRetirementIntent(data, a.Plan)
			if err != nil || partial.Completed != stop-1 || partial.Ready {
				t.Fatal(partial, err)
			}

			if phase, err := e.observeGatewayTLSRetirementLocked(ctx, a, partial); err != nil || phase != stop {
				t.Fatal("unexpected additional permission mutation", phase, err)
			}
			authority = true
			source := a.Plan
			if err := a.Close(); err != nil {
				t.Fatal(err)
			}
			if err := e.Close(); err != nil {
				t.Fatal(err)
			}
			e = openEngine(t, host, jr)
			defer e.Close()
			a, err = e.reopenGatewayTLSRetirementLocked(ctx, source)
			if err != nil {
				t.Fatal("reopen interrupted permission state", err)
			}
			defer a.Close()
			e.checkpoint = nil
			if err := e.applyGatewayTLSRetirementLocked(ctx, a, guard); err != nil {
				t.Fatal("resume", err)
			}
			if err := e.applyGatewayTLSRetirementLocked(ctx, a, guard); err != nil {
				t.Fatal("ready replay", err)
			}
			data, err = e.readGatewayTLSRetirementIntent()
			if err != nil {
				t.Fatal(err)
			}
			final, err := decodeGatewayTLSRetirementIntent(data, a.Plan)
			if err != nil || !final.Ready || final.Completed != 5 {
				t.Fatal(final, err)
			}
			if phase, err := e.observeGatewayTLSRetirementLocked(ctx, a, final); err != nil || phase != 5 {
				t.Fatal(phase, err)
			}

			// Identical contents at the same name cannot replace the recorded inode.
			if err := e.host.Rename(key, key+".retained-original"); err != nil {
				t.Fatal(err)
			}
			if err := e.host.WriteFile(key, []byte("retained key fixture"), 0600); err != nil {
				t.Fatal(err)
			}
			if replaced, err := e.reopenGatewayTLSRetirementLocked(ctx, source); err == nil {
				replaced.Close()
				t.Fatal("same-content replacement TLS inode admitted")
			}
		})
	}
}
