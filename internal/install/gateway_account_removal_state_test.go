package install

import (
	"strings"
	"testing"
)

func TestGatewayRemovalStateReconcilesOrderedRemoval(t *testing.T) {
	p, g, s := accountFixture()
	owner := strings.Repeat("a", 32)
	plan := gatewayRemovalPlan(owner, GatewayAccount{UID: 803, GID: 803, ProxyGID: 804})
	user := "homenode-gateway:x:803:803:HomeNode install " + owner + ":/nonexistent:/usr/sbin/nologin\n"
	shadow := "homenode-gateway:!:1:0:99999:7:::\n"
	private := "homenode-gateway:x:803:\n"
	proxy := "homenode-proxy:x:804:"
	snapshot := func(p, g, s string) accountSnapshot {
		return accountSnapshot{passwd: []byte(p), groups: []byte(g), shadow: []byte(s), nss: []byte("passwd: files systemd\ngroup: files systemd\nshadow: files\n")}
	}
	states := []accountSnapshot{
		snapshot(p+user, g+private+proxy+"homenode,homenode-gateway\n", s+shadow),
		snapshot(p+user, g+private+proxy+"homenode-gateway\n", s+shadow),
		snapshot(p, g+private+proxy+"\n", s),
		snapshot(p, g+private, s),
		snapshot(p, g, s),
	}
	for phase, current := range states {
		got, err := gatewayRemovalState(current, plan)
		if err != nil || got != phase {
			t.Fatal(phase, got, err)
		}
	}
	// userdel may already have removed the private group.
	if got, err := gatewayRemovalState(snapshot(p, g+proxy+"\n", s), plan); err != nil || got != 2 {
		t.Fatal(got, err)
	}
	bad := []accountSnapshot{
		snapshot(p+user, g+private+proxy+"homenode,foreign,homenode-gateway\n", s+shadow),
		snapshot(p+strings.Replace(user, owner, "foreign", 1), g+private+proxy+"homenode-gateway\n", s+shadow),
		snapshot(p, g+private+proxy+"homenode\n", s),
		snapshot(p, g+private+proxy+"homenode-gateway\n", s),
		snapshot(p+"foreign:x:803:1000::/home/foreign:/bin/sh\n", g, s),
		snapshot(p, g+"foreign:x:804:\n", s),
		snapshot(p, g, s+shadow),
		snapshot(p+user, g+private, s+shadow),
	}
	for i, current := range bad {
		if _, err := gatewayRemovalState(current, plan); err == nil {
			t.Fatal("unowned state admitted", i)
		}
	}
}
