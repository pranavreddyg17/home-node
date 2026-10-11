//go:build linux

package install

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestRootGatewayTLSRetirementResumesEveryPermissionAcknowledgement(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root-only temporary filesystem fixture")
	}
	for stop := 1; stop <= 5; stop++ {
		t.Run(string(rune('0'+stop)), func(t *testing.T) {
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
			guard := func() error { return nil }
			interrupted := errors.New("lost TLS permission acknowledgement")
			count := 0
			e.checkpoint = func(stage, name string) error {
				if stage == "gateway-tls-retired" {
					count++
					if count == stop {
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
		})
	}
}
