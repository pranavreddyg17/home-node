package supervisor

import (
	"errors"
	"strings"
	"testing"
)

func TestQEMUSandboxArguments(t *testing.T) {
	prefix := "/usr/bin/qemu-system-x86_64\x00-sandbox\x00"
	options := "on,obsolete=deny,elevateprivileges=deny,spawn=deny,resourcecontrol=deny"
	valid := prefix + options + "\x00"
	for _, args := range []string{valid, prefix + "spawn=deny,on,resourcecontrol=deny,elevateprivileges=deny,obsolete=deny\x00-name\x00generated\x00"} {
		if err := validateQEMUSandboxArguments([]byte(args)); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range []string{"", strings.TrimSuffix(valid, "\x00"), strings.Replace(valid, "/usr/bin/qemu-system-x86_64", "qemu", 1), strings.Replace(valid, "on,", "off,", 1), strings.Replace(valid, "spawn=deny", "spawn=allow", 1), strings.Replace(valid, "obsolete=deny,", "", 1), strings.Replace(valid, "obsolete=deny", "on", 1), strings.Replace(valid, "-sandbox", "--sandbox", 1), strings.Replace(valid, "-sandbox\x00", "-sandbox=", 1), valid + "-sandbox\x00" + options + "\x00", valid + "-sandbox\x00", valid + "--sandbox=off\x00", prefix + options + ",extra=deny\x00", strings.Replace(valid, "resourcecontrol=deny", "resourcecontrol=deny\n", 1), prefix + string([]byte{255}) + "\x00", valid + strings.Repeat("a", 128<<10) + "\x00", valid + strings.Repeat("a\x00", 1025)} {
		if err := validateQEMUSandboxArguments([]byte(args)); !errors.Is(err, ErrPolicy) {
			t.Fatal("unqualified sandbox arguments accepted", err)
		}
	}
}
