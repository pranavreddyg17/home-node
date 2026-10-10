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
