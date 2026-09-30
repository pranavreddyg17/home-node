//go:build linux

package guestmount

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestDescriptorMountIDMatchesActualKernelMount(t *testing.T) {
	fd, err := unix.Open(t.TempDir(), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fd)
	id, err := descriptorMountID(fd)
	if err != nil {
		t.Fatal(err)
	}
	var info unix.Stat_t
	if err := unix.Fstat(fd, &info); err != nil {
		t.Fatal(err)
	}
	device := fmt.Sprintf("%d:%d", unix.Major(uint64(info.Dev)), unix.Minor(uint64(info.Dev)))
	data, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 2 && fields[0] == id {
			count++
			if fields[2] != device {
				t.Fatal("descriptor mount refers to another filesystem device")
			}
		}
	}
	if count != 1 {
		t.Fatal("descriptor did not select exactly one kernel mount record")
	}
	if _, err := descriptorMountID(-1); err == nil {
		t.Fatal("invalid descriptor admitted")
	}
}
