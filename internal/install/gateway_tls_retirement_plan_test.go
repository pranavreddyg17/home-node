package install

import (
	"strings"
	"testing"
)

func TestGatewayTLSRetirementOrdersRootOnlyPermissionsBeforeGroupRelease(t *testing.T) {
	identity := GatewayAccount{UID: 803, GID: 803, ProxyGID: 804}
	installed := journal{ID: strings.Repeat("a", 32), Digest: strings.Repeat("b", 64), Phase: "installed", Items: []record{{Path: "etc/homenode/tls", Directory: true, UID: 0, GID: 803, Mode: 0750, State: "created"}}}
	key := ConfigurationRemovalFile{Path: "etc/homenode/tls/server.key", UID: 0, GID: 803, Mode: 0640, SHA256: strings.Repeat("c", 64)}
	cert := ConfigurationRemovalFile{Path: "etc/homenode/tls/server.crt", UID: 0, GID: 803, Mode: 0644, SHA256: strings.Repeat("d", 64)}
	plan, err := gatewayTLSRetirementPlan(installed, identity, key, cert)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Changes) != 5 || plan.Changes[0].AfterMode != 0600 || plan.Changes[0].AfterGID != 803 || plan.Changes[1].AfterGID != 0 || plan.Changes[1].BeforeMode != 0600 || plan.Changes[3].AfterMode != 0700 || plan.Changes[4].BeforeMode != 0700 || plan.Changes[4].AfterGID != 0 {
		t.Fatal("unsafe permission transition order", plan)
	}
	for _, bad := range []ConfigurationRemovalFile{
		{Path: "etc/shadow", UID: 0, GID: 803, Mode: 0640, SHA256: key.SHA256},
		{Path: key.Path, UID: 803, GID: 803, Mode: 0640, SHA256: key.SHA256},
		{Path: key.Path, UID: 0, GID: 800, Mode: 0640, SHA256: key.SHA256},
		{Path: key.Path, UID: 0, GID: 803, Mode: 0644, SHA256: key.SHA256},
	} {
		if _, err := gatewayTLSRetirementPlan(installed, identity, bad, cert); err == nil {
			t.Fatal("foreign TLS source admitted", bad)
		}
	}

	for _, malformed := range []string{"", strings.Repeat("z", 32), strings.Repeat("a", 31)} {
		bad := installed
		bad.ID = malformed
		if _, err := gatewayTLSRetirementPlan(bad, identity, key, cert); err == nil {
			t.Fatal("malformed installation identity admitted")
		}
	}
	for _, malformed := range []string{"", strings.Repeat("z", 64), strings.Repeat("a", 63)} {
		bad := installed
		bad.Digest = malformed
		if _, err := gatewayTLSRetirementPlan(bad, identity, key, cert); err == nil {
			t.Fatal("malformed installation digest admitted")
		}
	}
	installed.Items = append(installed.Items, record{Path: "var/lib/homenode/control", UID: 803, GID: 800})
	if _, err := gatewayTLSRetirementPlan(installed, identity, key, cert); err == nil {
		t.Fatal("unhandled retained identity reference admitted")
	}
}
