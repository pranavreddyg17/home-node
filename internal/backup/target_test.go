package backup

import (
	"errors"
	"strings"
	"testing"
)

func registeredTarget() Target {
	return Target{MountPath: "/media/backup drive", UUID: "abcd-1234", RepositoryID: strings.Repeat("a", 64)}
}
func TestRegisteredDriveAdmission(t *testing.T) {
	data := []byte("24 1 8:1 / / rw,relatime - ext4 /dev/sda1 rw\n25 24 8:17 / /media/backup\\040drive rw,nosuid,nodev - ext4 /dev/sdb1 rw\n")
	mounts, err := ParseMountInfo(data)
	if err != nil {
		t.Fatal(err)
	}
	if mounts[1].ID != 25 {
		t.Fatal("mount identity lost")
	}
	target := registeredTarget()
	if _, err = AdmitMount(target, mounts, Device{8, 17}); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func([]Mount){
		func(m []Mount) { m[1].Path = "/media/absent" },
		func(m []Mount) { m[1].Device = Device{8, 18} },
		func(m []Mount) { m[1].Writable = false },
		func(m []Mount) { m[1].Root = "/some/bind" },
		func(m []Mount) { m[1].Filesystem = "nfs" },
	} {
		copyMounts := append([]Mount(nil), mounts...)
		mutate(copyMounts)
		if _, err = AdmitMount(target, copyMounts, Device{8, 17}); !errors.Is(err, ErrTarget) {
			t.Fatal("mismatched target admitted", err)
		}
	}
	if _, err = AdmitMount(target, mounts, Device{8, 1}); err == nil {
		t.Fatal("host root disk admitted")
	}
	if _, err = AdmitMount(target, append(mounts, mounts[1]), Device{8, 17}); err == nil {
		t.Fatal("ambiguous mount admitted")
	}
}
func TestMalformedTargetAndMountInfo(t *testing.T) {
	target := registeredTarget()
	target.UUID = "../../sda"
	if err := target.Validate(); err == nil {
		t.Fatal("device path injection")
	}
	for _, data := range []string{"", "0 1 8:1 / / rw - ext4 /dev/sda1 rw", "invalid 1 8:1 / / rw - ext4 /dev/sda1 rw", "24 1 8:x / / rw - ext4 /dev/sda1 rw", "24 1 8:1 / /bad\\999path rw - ext4 /dev/sda1 rw", "24 1 8:1 / / rw - ext4"} {
		if _, err := ParseMountInfo([]byte(data)); err == nil {
			t.Fatal("malformed mount accepted", data)
		}
	}
}
func FuzzMountInfo(f *testing.F) {
	f.Add([]byte("24 1 8:1 / / rw - ext4 /dev/sda1 rw\n"))
	f.Fuzz(func(t *testing.T, data []byte) { _, _ = ParseMountInfo(data) })
}
