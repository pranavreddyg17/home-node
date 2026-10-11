package install

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGatewayTLSRetirementIntentPinsCanonicalSourceAndProgress(t *testing.T) {
	identity := GatewayAccount{UID: 803, GID: 803, ProxyGID: 804}
	installed := journal{ID: strings.Repeat("a", 32), Digest: strings.Repeat("b", 64), Phase: "installed", Items: []record{{Path: "etc/homenode/tls", Directory: true, UID: 0, GID: 803, Mode: 0750, State: "created"}}}
	key := ConfigurationRemovalFile{Path: "etc/homenode/tls/server.key", GID: 803, Mode: 0640, SHA256: strings.Repeat("c", 64)}
	cert := ConfigurationRemovalFile{Path: "etc/homenode/tls/server.crt", GID: 803, Mode: 0644, SHA256: strings.Repeat("d", 64)}
	plan, err := gatewayTLSRetirementPlan(installed, identity, key, cert)
	if err != nil {
		t.Fatal(err)
	}
	intent := gatewayTLSRetirementIntent{Version: 1, Plan: plan, Sources: []TLSRetirementInode{{Path: key.Path, Device: 9, Inode: 1, Size: 100}, {Path: cert.Path, Device: 9, Inode: 2, Size: 200}, {Path: "etc/homenode/tls", Device: 9, Inode: 3, Size: 4096}}}
	for phase := 0; phase <= 5; phase++ {
		intent.Completed = phase
		intent.Ready = phase == 5
		data, _ := json.Marshal(intent)
		decoded, err := decodeGatewayTLSRetirementIntent(data, plan)
		if err != nil || decoded.Completed != phase || decoded.Ready != intent.Ready {
			t.Fatal(phase, err)
		}
	}
	intent.Completed = 0
	intent.Ready = false
	data, _ := json.Marshal(intent)
	for _, bad := range [][]byte{
		append([]byte(" "), data...),
		[]byte(strings.Replace(string(data), "\"version\":1", "\"version\":1,\"version\":1", 1)),
		[]byte(strings.Replace(string(data), "\"version\":1", "\"version\":1,\"unknown\":true", 1)),
		[]byte(strings.Replace(string(data), "\"version\":1", "\"version\":2", 1)),
		[]byte(strings.Replace(string(data), "\"version\":1", "\"version\":1,\"completed\":6", 1)),
		[]byte(strings.Replace(string(data), "\"version\":1", "\"version\":1,\"ready\":true", 1)),
		[]byte(strings.Replace(string(data), strings.Repeat("c", 64), strings.Repeat("e", 64), 1)),
	} {
		if _, err := decodeGatewayTLSRetirementIntent(bad, plan); err == nil {
			t.Fatal("noncanonical or foreign intent admitted")
		}
	}
	for _, mutate := range []func(*gatewayTLSRetirementIntent){
		func(i *gatewayTLSRetirementIntent) { i.Sources[0].Inode = 0 },
		func(i *gatewayTLSRetirementIntent) { i.Sources[1].Inode = i.Sources[0].Inode },
		func(i *gatewayTLSRetirementIntent) { i.Sources[0].Size = 0 },
		func(i *gatewayTLSRetirementIntent) { i.Sources[0].Size = 16385 },
		func(i *gatewayTLSRetirementIntent) { i.Sources[2].Path = "etc/shadow" },
		func(i *gatewayTLSRetirementIntent) { i.Sources[2].Size = -1 },
	} {
		bad := intent
		bad.Sources = append([]TLSRetirementInode(nil), intent.Sources...)
		mutate(&bad)
		encoded, _ := json.Marshal(bad)
		if _, err := decodeGatewayTLSRetirementIntent(encoded, plan); err == nil {
			t.Fatal("foreign source identity admitted")
		}
	}
	if _, err := decodeGatewayTLSRetirementIntent(make([]byte, gatewayTLSRetirementJournalLimit+1), plan); err == nil {
		t.Fatal("unbounded intent admitted")
	}
}
