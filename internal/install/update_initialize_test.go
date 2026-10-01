package install

import "testing"

func TestUpdateBootstrapAdmissionRequiresCompleteOwnedJournal(t *testing.T) {
	fixture := func() journal {
		return journal{Phase: "installed", Items: []record{
			{Path: "etc/homenode/update-root.json", UID: 0, GID: 0, Mode: 0400, SHA256: digest([]byte("root fixture")), State: "created"},
			{Path: "var/lib/homenode-update", Directory: true, UID: 0, GID: 0, Mode: 0700, State: "created"},
			{Path: "var/lib/homenode-update/metadata", Directory: true, UID: 0, GID: 0, Mode: 0700, State: "created"},
			{Path: "var/lib/homenode-update/downloads", Directory: true, UID: 0, GID: 0, Mode: 0700, State: "created"},
		}}
	}
	for _, scenario := range []string{"valid", "incomplete", "missing", "duplicate", "public", "foreign-owner", "wrong-group", "writable-root", "adopted-root"} {
		t.Run(scenario, func(t *testing.T) {
			j := fixture()
			switch scenario {
			case "incomplete":
				j.Phase = "installing"
			case "missing":
				j.Items = j.Items[:3]
			case "duplicate":
				j.Items = append(j.Items, j.Items[0])
			case "public":
				j.Items[2].Mode = 0755
			case "foreign-owner":
				j.Items[2].UID = 800
			case "wrong-group":
				j.Items[2].GID = 800
			case "writable-root":
				j.Items[0].Mode = 0600
			case "adopted-root":
				j.Items[0].State = "existing"
			}
			record, err := updateBootstrapRecord(j, 0)
			if scenario == "valid" {
				if err != nil || record != j.Items[0] {
					t.Fatal(record, err)
				}
			} else if err == nil {
				t.Fatal("unowned bootstrap admitted")
			}
		})
	}
}
