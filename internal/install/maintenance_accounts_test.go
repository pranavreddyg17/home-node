package install

import (
	"strings"
	"testing"
)

func TestMaintenanceAccountIsolation(t *testing.T) {
	passwd, groups, shadow := accountFixture()
	passwd += "homenode-backup:x:803:803:Backup:/nonexistent:/usr/sbin/nologin\n"
	groups += "homenode-backup:x:803:\n"
	shadow += "homenode-backup:!:1:0:99999:7:::\n"
	identity, err := ValidateMaintenanceAccount([]byte(passwd), []byte(groups), []byte(shadow))
	if err != nil || identity.UID != 803 || identity.GID != 803 {
		t.Fatal(identity, err)
	}
	for _, fixture := range []struct{ name, p, g, s string }{
		{"root", strings.Replace(passwd, "backup:x:803", "backup:x:0", 1), groups, shadow},
		{"duplicate uid", passwd + "other:x:803:804::/nonexistent:/usr/sbin/nologin\n", groups, shadow},
		{"foreign primary group", passwd + "other:x:804:803::/nonexistent:/usr/sbin/nologin\n", groups, shadow},
		{"group alias", passwd, groups + "other:x:803:\n", shadow},
		{"duplicate private member", passwd, strings.Replace(groups, "backup:x:803:", "backup:x:803:homenode-backup,homenode-backup", 1), shadow},
		{"runtime membership", passwd, strings.Replace(groups, "homenode,homenode-transfer", "homenode,homenode-transfer,homenode-backup", 1), shadow},
		{"foreign membership", passwd, groups + "sudo:x:27:homenode-backup\n", shadow},
		{"private group foreign member", passwd, strings.Replace(groups, "backup:x:803:", "backup:x:803:homenode", 1), shadow},
		{"unlocked", passwd, groups, strings.Replace(shadow, "homenode-backup:!:", "homenode-backup:hash:", 1)},
		{"login shell", strings.Replace(passwd, "Backup:/nonexistent:/usr/sbin/nologin", "Backup:/nonexistent:/bin/bash", 1), groups, shadow},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			if _, err := ValidateMaintenanceAccount([]byte(fixture.p), []byte(fixture.g), []byte(fixture.s)); err == nil {
				t.Fatal("unsafe backup identity accepted")
			}
		})
	}
}
