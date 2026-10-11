package install

import (
	"reflect"
	"strings"
	"testing"
)

func TestGatewayTLSRetirementInstallationChangesOnlyRetainedDirectoryAuthority(t *testing.T) {
	identity := GatewayAccount{UID: 803, GID: 803, ProxyGID: 804}
	source := journal{Version: 1, ID: strings.Repeat("a", 32), Phase: "installed", Items: []record{
		{Path: "etc/homenode/tls", Directory: true, UID: 0, GID: 803, Mode: 0750, State: "created"},
		{Path: "var/lib/homenode", Directory: true, UID: 0, GID: 800, Mode: 0755, State: "created"},
	}}
	source.Digest = normalizedTLSInstallationDigest(source.Items)
	key := ConfigurationRemovalFile{Path: "etc/homenode/tls/server.key", GID: 803, Mode: 0640, SHA256: strings.Repeat("c", 64)}
	cert := ConfigurationRemovalFile{Path: "etc/homenode/tls/server.crt", GID: 803, Mode: 0644, SHA256: strings.Repeat("d", 64)}
	plan, err := gatewayTLSRetirementPlan(source, identity, key, cert)
	if err != nil {
		t.Fatal(err)
	}
	intent := gatewayTLSRetirementIntent{Version: 1, Plan: plan, Ready: true, Completed: 5}
	retired, err := gatewayTLSRetiredInstallation(source, intent)
	if err != nil {
		t.Fatal(err)
	}
	if retired.Items[0].GID != 0 || retired.Items[0].Mode != 0700 || retired.Items[1] != source.Items[1] || source.Items[0].GID != 803 || source.Items[0].Mode != 0750 || retired.ID != source.ID || retired.Digest == source.Digest {
		t.Fatal(source, retired)
	}
	restored, err := gatewayTLSSourceInstallation(retired, intent)
	if err != nil || !reflect.DeepEqual(restored, source) {
		t.Fatal(restored, err)
	}
	pending := intent
	pending.Ready = false
	if _, err := gatewayTLSRetiredInstallation(source, pending); err == nil {
		t.Fatal("unready permission transition admitted")
	}
	foreign := retired
	foreign.Items = append([]record(nil), retired.Items...)
	foreign.Items[1].GID = 801
	foreign.Digest = normalizedTLSInstallationDigest(foreign.Items)
	if _, err := gatewayTLSSourceInstallation(foreign, intent); err == nil {
		t.Fatal("unrelated retained ownership change admitted")
	}
	stale := source
	stale.Items = append([]record(nil), source.Items...)
	stale.Items[1].Mode = 0700
	if _, err := gatewayTLSRetiredInstallation(stale, intent); err == nil {
		t.Fatal("source digest drift admitted")
	}
}
