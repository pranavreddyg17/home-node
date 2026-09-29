package install

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func accountFixture() (string, string, string) {
	return "root:x:0:0:root:/root:/bin/bash\nhomenode:x:800:800:HomeNode:/nonexistent:/usr/sbin/nologin\nhomenode-transfer:x:801:801:Transfer:/nonexistent:/usr/sbin/nologin\n",
		"root:x:0:\nhomenode:x:800:\nhomenode-transfer:x:801:\nhomenode-runtime:x:802:homenode,homenode-transfer\nlibvirt-qemu:x:64055:libvirt-qemu\n",
		"root:!:1:0:99999:7:::\nhomenode:!:1:0:99999:7:::\nhomenode-transfer:*:1:0:99999:7:::\n"
}
func TestLocalServiceIdentityIsolation(t *testing.T) {
	passwd, group, shadow := accountFixture()
	actual, err := ValidateLocalAccounts([]byte(passwd), []byte(group), []byte(shadow))
	expected := Accounts{ControllerUID: 800, TransferUID: 801, ControllerGID: 800, TransferGID: 801, RuntimeGID: 802, QEMUGID: 64055}
	if err != nil || actual != expected {
		t.Fatal(actual, err)
	}
	cases := []struct {
		name                  string
		passwd, group, shadow string
	}{
		{"UID alias", passwd + "other:x:800:900:Other:/home/other:/bin/bash\n", group, shadow},
		{"shared service UID", strings.Replace(passwd, ":801:801:", ":800:801:", 1), group, shadow},
		{"interactive shell", strings.Replace(passwd, "/usr/sbin/nologin", "/bin/bash", 1), group, shadow},
		{"real home", strings.Replace(passwd, "/nonexistent", "/root", 1), group, shadow},
		{"human UID range", strings.Replace(passwd, ":800:800:", ":1000:800:", 1), group, shadow},
		{"non-shadow passwd credential", strings.Replace(passwd, "homenode:x:", "homenode:PRIVATE-HASH-MARKER:", 1), group, shadow},
		{"unlocked password", passwd, group, strings.Replace(shadow, "homenode:!:", "homenode:PRIVATE-HASH-MARKER:", 1)},
		{"empty password", passwd, group, strings.Replace(shadow, "homenode:!:", "homenode::", 1)},
		{"missing shadow", passwd, group, "root:!:1:0:99999:7:::\n"},
		{"foreign private membership", passwd, strings.Replace(group, "homenode:x:800:\n", "homenode:x:800:other\n", 1), shadow},
		{"foreign primary group", passwd + "other:x:900:800:Other:/home/other:/bin/bash\n", group, shadow},
		{"private group alias", passwd, group + "alias:x:800:other\n", shadow},
		{"privileged supplementary group", passwd, group + "libvirt:x:803:homenode\n", shadow},
		{"shared runtime with foreign account", passwd, strings.Replace(group, ":homenode,homenode-transfer", ":homenode,homenode-transfer,other", 1), shadow},
		{"missing runtime membership", passwd, strings.Replace(group, ":homenode,homenode-transfer", ":homenode-transfer", 1), shadow},
		{"QEMU group reuse", passwd, strings.Replace(group, "libvirt-qemu:x:64055:", "libvirt-qemu:x:802:", 1), shadow},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ValidateLocalAccounts([]byte(tc.passwd), []byte(tc.group), []byte(tc.shadow)); err == nil {
				t.Fatal("unsafe account configuration accepted")
			} else if strings.Contains(err.Error(), "PRIVATE-HASH-MARKER") {
				t.Fatal("shadow content exposed")
			}
		})
	}
	if _, err := ValidateLocalAccounts(bytes.Repeat([]byte("x"), maxAccountFileBytes+1), []byte(group), []byte(shadow)); err == nil {
		t.Fatal("unbounded database accepted")
	}
}
func TestNameResolutionRejectsRemoteIdentitySources(t *testing.T) {
	valid := "passwd: files systemd\ngroup: files systemd\nshadow: files\nhosts: files dns\n"
	if err := ValidateNameServices([]byte(valid)); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{
		strings.Replace(valid, "passwd: files systemd", "passwd: files sss", 1),
		strings.Replace(valid, "group: files systemd", "group: files ldap", 1),
		strings.Replace(valid, "shadow: files", "shadow: compat", 1),
		valid + "passwd: files\n",
		"passwd: files\ngroup: files\n",
		strings.Replace(valid, "passwd: files systemd", "passwd: systemd files", 1),
	} {
		if err := ValidateNameServices([]byte(invalid)); err == nil {
			t.Fatal("unsupported resolution accepted")
		}
	}
}

func TestAccountLookupCannotReadShadowOrAcceptOptions(t *testing.T) {
	for _, value := range []struct{ database, key string }{{"shadow", "homenode"}, {"group", "--help"}, {"passwd", "root\nother"}} {
		if _, _, err := lookupAccount(context.Background(), value.database, value.key); err == nil {
			t.Fatal("unsafe lookup accepted")
		}
	}
}
