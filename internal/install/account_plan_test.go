package install

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestAccountCreationPlanReservesDistinctUnoccupiedIDs(t *testing.T) {
	passwd := []byte("root:x:0:0:root:/root:/bin/bash\nother:x:999:998:Other:/nonexistent:/usr/sbin/nologin\n")
	groups := []byte("root:x:0:\nlibvirt-qemu:x:64055:libvirt-qemu\noccupied:x:999:\n")
	nss := []byte("passwd: files systemd\ngroup: files\nshadow: files\n")
	plan, err := PlanAccountCreation(strings.Repeat("a", 32), passwd, groups, nss)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Accounts.ControllerUID != 998 || plan.Accounts.TransferUID != 997 || plan.Accounts.ControllerGID != 997 || plan.Accounts.TransferGID != 996 || plan.Accounts.RuntimeGID != 995 {
		t.Fatal("occupied or dangling IDs reused", plan.Accounts)
	}
	if len(plan.Commands) != 5 || len(plan.Pending) == 0 {
		t.Fatal("incomplete creation plan")
	}
	for i, command := range plan.Commands {
		if (i < 3 && command.Program != "/usr/sbin/groupadd") || (i >= 3 && command.Program != "/usr/sbin/useradd") {
			t.Fatal("unapproved command")
		}
		for _, argument := range command.Arguments {
			if strings.ContainsAny(argument, "\n\r\x00") {
				t.Fatal("unsafe command argument")
			}
		}
	}
	again, err := PlanAccountCreation(plan.OwnerID, passwd, groups, nss)
	if err != nil || again.Accounts != plan.Accounts {
		t.Fatal("non-deterministic ID reservation", err)
	}
}
func TestAccountCreationRefusesForeignNamesAndStaleMembership(t *testing.T) {
	passwd := []byte("root:x:0:0:root:/root:/bin/bash\n")
	groups := []byte("root:x:0:\nlibvirt-qemu:x:64055:libvirt-qemu\n")
	nss := []byte("passwd: files\ngroup: files\nshadow: files\n")
	owner := strings.Repeat("a", 32)
	for _, name := range []string{"homenode", "homenode-transfer"} {
		occupied := append(append([]byte{}, passwd...), []byte(fmt.Sprintf("%s:x:800:800:Service:/nonexistent:/usr/sbin/nologin\n", name))...)
		if _, err := PlanAccountCreation(owner, occupied, groups, nss); err == nil {
			t.Fatal("foreign user adopted", name)
		}
		stale := append(append([]byte{}, groups...), []byte("other:x:800:"+name+"\n")...)
		if _, err := PlanAccountCreation(owner, passwd, stale, nss); err == nil {
			t.Fatal("stale membership inherited", name)
		}
	}
	for _, name := range []string{"homenode", "homenode-transfer", "homenode-runtime"} {
		occupied := append(append([]byte{}, groups...), []byte(name+":x:800:\n")...)
		if _, err := PlanAccountCreation(owner, passwd, occupied, nss); err == nil {
			t.Fatal("foreign group adopted", name)
		}
	}
	if _, err := PlanAccountCreation("invalid\ncomment", passwd, groups, nss); err == nil {
		t.Fatal("unsafe ownership marker accepted")
	}
	exhausted := append([]byte{}, passwd...)
	for id := 100; id <= 999; id++ {
		exhausted = append(exhausted, []byte(fmt.Sprintf("u%d:x:%d:0:User:/nonexistent:/usr/sbin/nologin\n", id, id))...)
	}
	if _, err := PlanAccountCreation(owner, exhausted, groups, nss); err == nil {
		t.Fatal("exhausted namespace accepted")
	}
	alias := append(append([]byte{}, groups...), []byte("alias:x:64055:\n")...)
	if _, err := PlanAccountCreation(owner, passwd, alias, nss); err == nil {
		t.Fatal("QEMU group alias accepted")
	}
	if _, err := PlanAccountCreation(owner, passwd, bytes.ReplaceAll(groups, []byte("libvirt-qemu"), []byte("other")), nss); err == nil {
		t.Fatal("missing QEMU dependency accepted")
	}
}
