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

func TestGuestUnifiedMembershipRefusesAmbiguity(t *testing.T) {
	id := strings.Repeat("A", 26)
	relative := `/homenode.slice/machine-qemu\x2d1\x2dhomenode\x2d` + id + ".scope/libvirt/emulator"
	for _, input := range []string{"0::" + relative, "0::" + relative + "\n"} {
		got, err := guestUnifiedMembership([]byte(input), id)
		if err != nil || got != relative {
			t.Fatal("unified membership refused", got, err)
		}
	}
	for _, input := range []string{"", "0::" + relative + "\n\n", "0::" + relative + "\n0::" + relative, "1:memory:" + relative, "0::" + relative + "\r\n", "0::" + relative + " (deleted)", "0::" + relative + "\x00", "0::/homenode.slice", strings.Repeat("X", 4097)} {
		if got, err := guestUnifiedMembership([]byte(input), id); err == nil || got != "" {
			t.Fatalf("ambiguous membership admitted %q", input)
		}
	}
}
