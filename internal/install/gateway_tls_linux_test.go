//go:build linux

package install

import (
	"os"
	"path/filepath"
	"testing"
)

// Permission fixture only: this does not qualify certificate trust or systemd.
func TestRootGatewayTLSKeyRefusesControllerGroup(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root-only temporary Linux fixture")
	}
	e, c, host, _, _, _ := imagePlacementFixture(t)
	defer e.Close()
	gateway := GatewayAccount{UID: 803, GID: 803, ProxyGID: 804}
	cert := filepath.Join(host, "etc/homenode/tls/server.crt")
	key := filepath.Join(host, "etc/homenode/tls/server.key")
	if err := os.WriteFile(cert, []byte("permission fixture"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(key, []byte("permission fixture"), 0640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(key, 0, c.Accounts.ControllerGID); err != nil {
		t.Fatal(err)
	}
	if err := e.checkTLSAccessWithGateway(c.Accounts, &gateway); err == nil {
		t.Fatal("controller-owned key admitted for gateway")
	}
	if err := os.Chown(key, 0, gateway.GID); err != nil {
		t.Fatal(err)
	}
	if err := e.checkTLSAccessWithGateway(c.Accounts, &gateway); err != nil {
		t.Fatal("gateway key group refused", err)
	}
	if err := e.checkTLSAccess(c.Accounts); err == nil {
		t.Fatal("legacy check accepted gateway group")
	}
	if err := os.Chmod(key, 0644); err != nil {
		t.Fatal(err)
	}
	if err := e.checkTLSAccessWithGateway(c.Accounts, &gateway); err == nil {
		t.Fatal("world-readable gateway key accepted")
	}
}
