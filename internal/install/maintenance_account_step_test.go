package install

import (
	"strings"
	"testing"
)

func TestMaintenanceAccountStepsReconcileOnlyOwnedIdentities(t *testing.T) {
	p, g, s := accountFixture()
	nss := []byte("passwd: files\ngroup: files\nshadow: files\n")
	plan := MaintenanceAccountPlan{OwnerID: strings.Repeat("a", 32), Identity: MaintenanceAccount{UID: 803, GID: 803}}
	snapshot := accountSnapshot{passwd: []byte(p), groups: []byte(g), shadow: []byte(s), nss: nss}
	if ready, err := maintenanceAccountStepMatches(snapshot, plan, 0); ready || err != nil {
		t.Fatal(ready, err)
	}
	snapshot.groups = append(snapshot.groups, []byte("homenode-backup:x:803:\n")...)
	if ready, err := maintenanceAccountStepMatches(snapshot, plan, 0); !ready || err != nil {
		t.Fatal("created group forgotten", ready, err)
	}
	if ready, err := maintenanceAccountStepMatches(snapshot, plan, 1); ready || err != nil {
		t.Fatal("missing user accepted", ready, err)
	}
	snapshot.passwd = append(snapshot.passwd, []byte("homenode-backup:x:803:803:HomeNode install "+plan.OwnerID+":/nonexistent:/usr/sbin/nologin\n")...)
	snapshot.shadow = append(snapshot.shadow, []byte("homenode-backup:!:1:0:99999:7:::\n")...)
	if ready, err := maintenanceAccountStepMatches(snapshot, plan, 1); !ready || err != nil {
		t.Fatal("owned user forgotten", ready, err)
	}
	for _, kind := range []string{"marker", "uid-alias", "group-alias", "extra-group", "unlocked"} {
		t.Run(kind, func(t *testing.T) {
			changed := snapshot
			switch kind {
			case "marker":
				changed.passwd = []byte(strings.Replace(string(snapshot.passwd), "HomeNode install ", "Foreign install ", 1))
			case "uid-alias":
				changed.passwd = append(append([]byte{}, snapshot.passwd...), []byte("other:x:803:804::/nonexistent:/usr/sbin/nologin\n")...)
			case "group-alias":
				changed.groups = append(append([]byte{}, snapshot.groups...), []byte("other:x:803:\n")...)
			case "extra-group":
				changed.groups = append(append([]byte{}, snapshot.groups...), []byte("sudo:x:27:homenode-backup\n")...)
			case "unlocked":
				changed.shadow = []byte(strings.Replace(string(snapshot.shadow), "homenode-backup:!:", "homenode-backup:hash:", 1))
			}
			if ready, err := maintenanceAccountStepMatches(changed, plan, 1); ready || err == nil {
				t.Fatal("foreign or unsafe identity accepted", ready, err)
			}
		})
	}
}
