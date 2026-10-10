package install

import (
	"reflect"
	"strings"
	"testing"
)

func TestGatewayPlanReservesIsolatedIDsAndRefusesAdoption(t *testing.T) {
	p, g, s := accountFixture()
	nss := []byte("passwd: files systemd\ngroup: files systemd\nshadow: files\n")
	owner := strings.Repeat("a", 32)
	plan, err := PlanGatewayAccountCreation(owner, []byte(p), []byte(g), []byte(s), nss)
	if err != nil || plan.Identity != (GatewayAccount{UID: 999, GID: 999, ProxyGID: 998}) || len(plan.Commands) != 4 {
		t.Fatal(plan, err)
	}
	if !reflect.DeepEqual(plan.Commands[3].Arguments, []string{"--append", "--groups", "homenode-proxy", "homenode"}) {
		t.Fatal("controller membership would replace runtime membership")
	}
	if strings.Contains(strings.Join(plan.Commands[2].Arguments, " "), "homenode-runtime") {
		t.Fatal("gateway received runtime authority")
	}
	for _, name := range []string{"homenode-gateway", "homenode-proxy"} {
		for _, fixture := range [][3]string{
			{p + name + ":x:900:900::/nonexistent:/usr/sbin/nologin\n", g, s},
			{p, g + name + ":x:900:\n", s},
			{p, g + "other:x:900:" + name + "\n", s},
			{p, g, s + name + ":!:1:0:99999:7:::\n"},
		} {
			if _, err := PlanGatewayAccountCreation(owner, []byte(fixture[0]), []byte(fixture[1]), []byte(fixture[2]), nss); err == nil {
				t.Fatal("occupied or dangling name adopted", name)
			}
		}
	}
	plan, err = PlanGatewayAccountCreation(owner, []byte(p+"other:x:999:998::/nonexistent:/usr/sbin/nologin\n"), []byte(g+"other:x:999:\n"), []byte(s), nss)
	if err != nil || plan.Identity != (GatewayAccount{UID: 998, GID: 997, ProxyGID: 996}) {
		t.Fatal("occupied or dangling IDs reused", plan, err)
	}
}
