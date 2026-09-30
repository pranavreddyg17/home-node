package guestmount

import (
	"strings"
	"testing"
)

func TestDataMountRequiresNamedDeviceAndConfinement(t *testing.T) {
	valid := "31 22 253:16 / /data rw,nosuid,nodev,noexec,relatime - ext4 /dev/vdb rw\n"
	if err := admit(valid, "253:16"); err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{
		"", valid + valid,
		strings.Replace(valid, "253:16", "253:0", 1),
		strings.Replace(valid, " / /data", " /subdirectory /data", 1),
		strings.Replace(valid, "/data", "/different", 1),
		strings.Replace(valid, "ext4", "tmpfs", 1),
		strings.Replace(valid, "nodev,", "", 1),
		strings.Replace(valid, "nosuid,", "", 1),
		strings.Replace(valid, "noexec,", "", 1),
		strings.Replace(valid, " rw\n", " ro\n", 1),
		strings.Replace(valid, "rw,nosuid", "ro,nosuid", 1),
		valid + "malformed\n",
		valid + strings.Repeat(" ", 1<<20),
	} {
		if err := admit(data, "253:16"); err == nil {
			t.Fatal("unsafe mount admitted")
		}
	}
}
