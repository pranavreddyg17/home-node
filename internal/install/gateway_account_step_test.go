package install

import (
	"strings"
	"testing"
)

func TestGatewayLostAcknowledgementRequiresOwnerAndIsolation(t *testing.T) {
	p, g, s := accountFixture()
	owner := strings.Repeat("a", 32)
	plan := GatewayAccountPlan{OwnerID: owner, Identity: GatewayAccount{UID: 803, GID: 803, ProxyGID: 804}}
	nss := []byte("passwd: files systemd\ngroup: files systemd\nshadow: files\n")
	snapshot := func(p, g, s string) accountSnapshot {
		return accountSnapshot{passwd: []byte(p), groups: []byte(g), shadow: []byte(s), nss: nss}
	}
	phases := []accountSnapshot{
		snapshot(p, g, s),
		snapshot(p, g+"homenode-gateway:x:803:\n", s),
		snapshot(p, g+"homenode-gateway:x:803:\nhomenode-proxy:x:804:\n", s),
		snapshot(p+"homenode-gateway:x:803:803:HomeNode install "+owner+":/nonexistent:/usr/sbin/nologin\n", g+"homenode-gateway:x:803:\nhomenode-proxy:x:804:homenode-gateway\n", s+"homenode-gateway:!:1:0:99999:7:::\n"),
	}
	full := phases[3]
	full.groups = []byte(strings.Replace(string(full.groups), "804:homenode-gateway", "804:homenode,homenode-gateway", 1))
	phases = append(phases, full)
	for phase, current := range phases {
		for step := 0; step < 4; step++ {
			matched, err := gatewayAccountStepMatches(current, plan, step)
			if err != nil || matched != (step < phase) {
				t.Fatal("lost acknowledgement reconciliation", phase, step, matched, err)
			}
		}
	}
	for _, bad := range []accountSnapshot{
		snapshot(strings.Replace(string(full.passwd), "HomeNode install "+owner, "foreign", 1), string(full.groups), string(full.shadow)),
		snapshot(string(full.passwd), string(full.groups)+"backup:x:805:homenode-gateway\n", string(full.shadow)),
		snapshot(string(full.passwd), string(full.groups)+"alias:x:804:\n", string(full.shadow)),
		snapshot(string(full.passwd), string(full.groups), strings.Replace(string(full.shadow), "homenode-gateway:!:", "homenode-gateway:hash:", 1)),
		snapshot(p, string(phases[2].groups)+"other:x:805:homenode-gateway\n", s),
	} {
		if _, err := gatewayAccountStepMatches(bad, plan, 3); err == nil {
			t.Fatal("unowned or contaminated partial state accepted")
		}
	}
}
