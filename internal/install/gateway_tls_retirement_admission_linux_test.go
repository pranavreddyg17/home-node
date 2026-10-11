//go:build linux

package install

import (
	"context"
	"os"
	"testing"
)

func TestRootGatewayTLSRetirementAdmissionPinsContentsAndRejectsHardlinks(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root-only temporary filesystem fixture")
	}
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
	if err := e.Apply(context.Background(), preview.Plan); err != nil {
		t.Fatal(err)
	}
	keyPath := "etc/homenode/tls/server.key"
	certPath := "etc/homenode/tls/server.crt"
	if err := e.host.WriteFile(keyPath, []byte("bounded private key fixture"), 0640); err != nil {
		t.Fatal(err)
	}
	if err := e.host.Chown(keyPath, 0, identity.GID); err != nil {
		t.Fatal(err)
	}
	if err := e.host.WriteFile(certPath, []byte("bounded public certificate fixture"), 0644); err != nil {
		t.Fatal(err)
	}
	a, err := e.admitGatewayTLSRetirementLocked(context.Background(), identity)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if len(a.Sources) != 3 {
		t.Fatal(a.Sources)
	}

	intent := gatewayTLSRetirementIntent{Version: 1, Plan: a.Plan, Sources: a.Sources}
	guard := func() error { return e.verifyGatewayTLSAdmissionLocked(context.Background(), a) }
	for replay := 0; replay < 2; replay++ {
		if err := e.prepareGatewayTLSRetirementIntentLocked(context.Background(), intent, guard); err != nil {
			t.Fatal("artifact-only intent preparation", err)
		}
	}
	data, err := e.readGatewayTLSRetirementIntent()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeGatewayTLSRetirementIntent(data, a.Plan); err != nil {
		t.Fatal(err)
	}
	if err := e.host.WriteFile(keyPath, []byte("changed private key fixture"), 0640); err != nil {
		t.Fatal(err)
	}
	if err := e.verifyGatewayTLSAdmissionLocked(context.Background(), a); err == nil {
		t.Fatal("changed retained contents admitted")
	}
	if err := os.Link(host+"/"+keyPath, host+"/etc/homenode/tls/key-alias"); err != nil {
		t.Fatal(err)
	}
	if admission, err := e.admitGatewayTLSRetirementLocked(context.Background(), identity); err == nil {
		admission.Close()
		t.Fatal("hardlinked key admitted")
	}
}
