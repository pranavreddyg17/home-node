package supervisor

import (
	"strings"
	"testing"
)

func TestGuestCgroupScopeBindsDomain(t *testing.T) {
	id := strings.Repeat("A", 26)
	scope := `/homenode.slice/machine-qemu\x2d1\x2dhomenode\x2d` + id + ".scope"
	for _, input := range []string{scope, scope + "/libvirt", scope + "/libvirt/emulator", scope + "/libvirt/vcpu0"} {
		got, err := guestCgroupScope(input, id)
		if err != nil || got != scope {
			t.Fatalf("domain scope refused: %q %v", got, err)
		}
	}
	for _, input := range []string{"/homenode.slice", scope + "/../other", scope + "/", scope + "/libvirt\n", strings.Replace(scope, "qemu\\x2d1", "qemu\\x2d01", 1), strings.Replace(scope, id, strings.Repeat("B", 26), 1), strings.Replace(scope, "homenode.slice", "other.slice", 1), scope + strings.Repeat("/child", 17)} {
		if got, err := guestCgroupScope(input, id); err == nil || got != "" {
			t.Fatalf("unbound scope admitted: %q", input)
		}
	}
	if _, err := guestCgroupScope(scope, "../bad"); err == nil {
		t.Fatal("invalid domain admitted")
	}
}
