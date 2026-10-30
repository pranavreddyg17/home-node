package supervisor

import (
	"errors"
	"strings"
	"testing"
)

func TestLocalLibvirtQEMUIdentity(t *testing.T) {
	valid := "root:x:0:0:root:/root:/bin/sh\nlibvirt-qemu:x:64055:993:Libvirt:/var/lib/libvirt:/usr/sbin/nologin\n"
	uid, gid, err := localLibvirtQEMUIdentity([]byte(valid))
	if err != nil || uid != 64055 || gid != 993 {
		t.Fatal(uid, gid, err)
	}
	for _, data := range []string{"", strings.Split(valid, "libvirt-qemu")[0], valid + "libvirt-qemu:x:107:993::/:/bin/false\n", valid + "alias:x:64055:993::/:/bin/false\n", strings.Replace(valid, "64055:993", "0:993", 1), strings.Replace(valid, "64055:993", "65536:993", 1), strings.Replace(valid, "64055:993", "64055:0", 1), strings.Replace(valid, "64055:993", "064055:993", 1), strings.Replace(valid, "64055:993", "64055:bad", 1), strings.Replace(valid, "64055:993", "64055:2147483648", 1), valid + "broken\n", valid + "\r", valid + "\x00", string([]byte{255}), strings.Repeat("x", (1<<20)+1)} {
		if _, _, err := localLibvirtQEMUIdentity([]byte(data)); !errors.Is(err, ErrPolicy) {
			t.Fatal("unqualified local service mapping accepted", err)
		}
	}
}

func TestDomainPIDDirectoryIdentity(t *testing.T) {
	for _, uid := range []uint32{0, 64055} {
		if err := qualifyDomainPIDDirectory(uid, 993, 0040000|0755, 2000000000, 64055, 993); err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []struct{ uid, gid, mode, guest, serviceUID, serviceGID uint32 }{
		{1000, 993, 0040000 | 0755, 2000000000, 64055, 993},
		{64055, 994, 0040000 | 0755, 2000000000, 64055, 993},
		{64055, 993, 0040000 | 0775, 2000000000, 64055, 993},
		{0, 0, 0040000 | 0777, 2000000000, 64055, 993},
		{64055, 993, 0040000 | 0755, 64055, 64055, 993},
		{64055, 993, 0040000 | 0755, 2000000000, 0, 993},
		{2000000000, 993, 0040000 | 0755, 2000000000, 2000000000, 993},
		{64055, 993, 0100000 | 0755, 2000000000, 64055, 993},
	} {
		if err := qualifyDomainPIDDirectory(test.uid, test.gid, test.mode, test.guest, test.serviceUID, test.serviceGID); !errors.Is(err, ErrPolicy) {
			t.Fatal("unqualified runtime directory accepted", test, err)
		}
	}
}
