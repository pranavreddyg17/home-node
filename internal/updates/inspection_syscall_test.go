package updates

import "testing"

func TestInspectionSyscallFilterRequiresIndependentCompleteDenyset(t *testing.T) {
	required := []string{"mount", "umount2", "reboot"}
	if err := ValidateInspectionSyscallFilter([]byte("SystemCallFilter=~reboot mount umount2\n"), required); err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{"SystemCallFilter=mount umount2 reboot\n", "SystemCallFilter=~mount reboot\n", "SystemCallFilter=~mount umount2 reboot mount\n", "SystemCallFilter=~mount umount2 reboot chroot\n", "SystemCallFilter=~@mount @reboot\n", "SystemCallFilter=~mount:EPERM umount2 reboot\n", "SystemCallFilter=~mount umount2 reboot\nSystemCallFilter=~mount\n"} {
		if err := ValidateInspectionSyscallFilter([]byte(data), required); err == nil {
			t.Fatal("unsafe denyset accepted", data)
		}
	}
	for _, profile := range [][]string{nil, {}, {"mount", "mount"}, {"@mount"}, {"mount:EPERM"}, {"1mount"}} {
		if err := ValidateInspectionSyscallFilter([]byte("SystemCallFilter=~mount\n"), profile); err == nil {
			t.Fatal("unqualified profile accepted", profile)
		}
	}
}
