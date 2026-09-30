//go:build linux

package guestmount

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// Check reads kernel mount/device evidence; it never formats, mounts or repairs.
func Check() error {
	var native unix.Stat_t
	if unix.Stat("/dev/disk/by-id/virtio-homenode-data", &native) != nil || native.Mode&unix.S_IFMT != unix.S_IFBLK {
		return ErrMount
	}
	identity := fmt.Sprintf("%d:%d", unix.Major(uint64(native.Rdev)), unix.Minor(uint64(native.Rdev)))
	readonly, err := os.Open("/sys/dev/block/" + identity + "/ro")
	if err != nil {
		return ErrMount
	}
	flag, err := io.ReadAll(io.LimitReader(readonly, 17))
	readonly.Close()
	if err != nil || strings.TrimSpace(string(flag)) != "0" {
		return ErrMount
	}
	serial, err := os.Open("/sys/dev/block/" + identity + "/serial")
	if err != nil {
		return ErrMount
	}
	value, err := io.ReadAll(io.LimitReader(serial, 257))
	serial.Close()
	if err != nil || len(value) > 256 || strings.TrimSpace(string(value)) != "homenode-data" {
		return ErrMount
	}
	dataFD, err := unix.Open("/data", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return ErrMount
	}
	defer unix.Close(dataFD)
	var mounted unix.Stat_t
	if unix.Fstat(dataFD, &mounted) != nil || mounted.Dev != native.Rdev {
		return ErrMount
	}
	fdinfo, err := os.Open("/proc/self/fdinfo/" + strconv.Itoa(dataFD))
	if err != nil {
		return ErrMount
	}
	descriptor, err := io.ReadAll(io.LimitReader(fdinfo, 4097))
	fdinfo.Close()
	if err != nil || len(descriptor) > 4096 {
		return ErrMount
	}
	mountID := ""
	for _, line := range strings.Split(string(descriptor), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == "mnt_id:" {
			if len(fields) != 2 || mountID != "" {
				return ErrMount
			}
			number, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil || number == 0 || strconv.FormatUint(number, 10) != fields[1] {
				return ErrMount
			}
			mountID = fields[1]
		}
	}
	if mountID == "" {
		return ErrMount
	}
	mounts, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return ErrMount
	}
	data, err := io.ReadAll(io.LimitReader(mounts, (1<<20)+1))
	mounts.Close()
	if err != nil {
		return ErrMount
	}
	return admit(string(data), identity, mountID)
}
