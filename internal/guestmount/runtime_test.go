package guestmount

import (
	"strings"
	"testing"
)

func TestRuntimeMountVisibleIdentity(t *testing.T) {
	good := "47 22 0:42 / /tmp rw,nosuid,nodev,noexec - tmpfs tmpfs rw,size=65536k,nr_inodes=8192\n"
	if admitRuntimeMount(good, "/tmp", "0:42", "47") != nil {
		t.Fatal("valid mount refused")
	}
	private := strings.Replace(good, "/ /tmp", "/systemd-private-fixture/tmp /tmp", 1)
	if admitRuntimeMount(private, "/tmp", "0:42", "47") != nil {
		t.Fatal("bounded private temporary bind refused")
	}
	varMount := strings.Replace(good, "/tmp", "/var", 1)
	if admitRuntimeMount(strings.Replace(varMount, "/ /var", "/sub /var", 1), "/var", "0:42", "47") == nil {
		t.Fatal("var subdirectory bind accepted")
	}
	hidden := strings.Replace(good, "47 22", "46 22", 1)
	if admitRuntimeMount(hidden+good, "/tmp", "0:42", "47") != nil {
		t.Fatal("hidden mount prevented admission")
	}
	for _, bad := range []string{"", good + good, strings.Replace(good, "0:42", "0:43", 1), strings.Replace(good, "/tmp", "/var", 1), strings.Replace(good, "- tmpfs", "- ext4", 1), strings.Repeat("x", (1<<20)+1)} {
		if admitRuntimeMount(bad, "/tmp", "0:42", "47") == nil {
			t.Fatal("unsafe mount accepted")
		}
	}
}
