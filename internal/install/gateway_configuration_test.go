package install

import (
	"bytes"
	servicetemplates "github.com/pranavreddyg17/home-node/packaging/systemd"
	"testing"
)

func TestGatewayControllerTransformationPreservesMaintenance(t *testing.T) {
	original, err := servicetemplates.Unit("homenode-control.service")
	if err != nil {
		t.Fatal(err)
	}
	private, err := gatewayControlUnit(original)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(private, []byte("--tls-key")) || !bytes.Contains(private, []byte("RestrictAddressFamilies=AF_UNIX\n")) || !bytes.Contains(private, []byte("InaccessiblePaths=-/etc/homenode/tls ")) {
		t.Fatal("controller retained TLS or IP authority")
	}
	private, err = maintenanceControlUnit(private, MaintenanceAccount{UID: 805, GID: 805})
	if err != nil {
		t.Fatal("private gateway broke backup activation", err)
	}
	if !bytes.Contains(private, []byte("--gateway-uid ${GATEWAY_UID}")) || !bytes.Contains(private, []byte("--maintenance-uid 805 --maintenance-gid 805")) {
		t.Fatal("identity options lost during composition")
	}
	if _, err := gatewayControlUnit(private); err == nil {
		t.Fatal("repeated transformation accepted")
	}
	if _, err := gatewayControlUnit(append(original, original...)); err == nil {
		t.Fatal("ambiguous source accepted")
	}
}

func TestGatewayConfigurationPlanBindsKeyDirectoryAndPeerIdentities(t *testing.T) {
	c, _, _, now := configurationFixture(t)
	c.Gateway = &GatewayAccount{UID: 803, GID: 803, ProxyGID: 804}
	c.Maintenance = &MaintenanceAccount{UID: 805, GID: 805}
	result, err := ConfigurationPlan(c, now)
	if err != nil {
		t.Fatal(err)
	}
	tls := findConfiguration(t, result.Plan, "etc/homenode/tls")
	if tls.UID != 0 || tls.GID != 803 || tls.Mode != 0750 {
		t.Fatal("controller can traverse gateway key directory", tls)
	}
	env := findConfiguration(t, result.Plan, "etc/homenode/services.env")
	if !bytes.Contains(env.Data, []byte("GATEWAY_UID=803\nPROXY_GID=804\n")) {
		t.Fatal("proxy peer IDs absent")
	}
	control := findConfiguration(t, result.Plan, "etc/systemd/system/homenode-control.service")
	if bytes.Contains(control.Data, []byte("--tls-key")) || !bytes.Contains(control.Data, []byte("--maintenance-uid 805")) {
		t.Fatal("controller authority or maintenance changed")
	}
	findConfiguration(t, result.Plan, "etc/systemd/system/homenode-gateway.service")
	for _, identity := range []GatewayAccount{{UID: 800, GID: 803, ProxyGID: 804}, {UID: 803, GID: 802, ProxyGID: 804}, {UID: 803, GID: 803, ProxyGID: 803}, {UID: 805, GID: 803, ProxyGID: 804}} {
		c.Gateway = &identity
		if _, err := ConfigurationPlan(c, now); err == nil {
			t.Fatal("gateway role alias accepted", identity)
		}
	}
}
