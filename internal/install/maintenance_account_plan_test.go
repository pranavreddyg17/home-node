package install

import (
	"strings"
	"testing"
)

func TestMaintenanceAccountPlanRefusesAdoptionAndDanglingAuthority(t *testing.T) {
	p, g, s := accountFixture()
	nss := []byte("passwd: files systemd\ngroup: files systemd\nshadow: files\n")
	owner := strings.Repeat("a", 32)
	plan, err := PlanMaintenanceAccountCreation(owner, []byte(p), []byte(g), []byte(s), nss)
	if err != nil || plan.Identity.UID != 999 || plan.Identity.GID != 999 || len(plan.Commands) != 2 {
		t.Fatal(plan, err)
	}
	if strings.Contains(strings.Join(plan.Commands[1].Arguments, " "), "--groups") {
		t.Fatal("backup gets supplementary authority")
	}
	for _, fixture := range [][3]string{
		{p + "homenode-backup:x:900:900::/nonexistent:/usr/sbin/nologin\n", g, s},
		{p, g + "homenode-backup:x:900:\n", s},
		{p, g + "other:x:900:homenode-backup\n", s},
		{p, g, s + "homenode-backup:!:1:0:99999:7:::\n"},
	} {
		if _, err := PlanMaintenanceAccountCreation(owner, []byte(fixture[0]), []byte(fixture[1]), []byte(fixture[2]), nss); err == nil {
			t.Fatal("occupied name adopted")
		}
	}
	plan, err = PlanMaintenanceAccountCreation(owner, []byte(p+"other:x:999:998::/nonexistent:/usr/sbin/nologin\n"), []byte(g+"other:x:999:\n"), []byte(s), nss)
	if err != nil || plan.Identity.UID != 998 || plan.Identity.GID != 997 {
		t.Fatal("occupied or dangling ID reused", plan, err)
	}
}
