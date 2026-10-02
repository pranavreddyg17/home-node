//go:build linux

package main

import (
	"bytes"
	"context"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestWorkerRefusesMissingOrUnexpectedArguments(t *testing.T) {
	for _, args := range [][]string{nil, {"--package", "/tmp/package.deb"}, {"--release", "0.1.0", "extra"}, {"--release", ""}} {
		var output bytes.Buffer
		if err := run(context.Background(), args, &output); err == nil || output.Len() != 0 {
			t.Fatal("invalid worker invocation accepted", args, err)
		}
	}
}

func TestWorkerArgumentBoundsBeforeHostAdmission(t *testing.T) {
	operation := "inspection-fixture-000001"
	if !validWorkerArguments(operation, "0.1.0~ci") {
		t.Fatal("valid launcher identity refused")
	}
	for _, pair := range [][2]string{{operation, ""}, {operation, "release"}, {operation, "0.1.0\n"}, {operation, strings.Repeat("1", 65)}, {strings.Repeat("a", 65), "0.1.0"}, {strings.Repeat("a", 1<<20), "0.1.0"}, {operation, strings.Repeat("1", 1<<20)}} {
		if validWorkerArguments(pair[0], pair[1]) {
			t.Fatal("invalid worker identity admitted")
		}
	}
}

func TestInspectionDescriptorRequiresReadableReadOnlyAuthority(t *testing.T) {
	path := filepath.Join(t.TempDir(), "package.deb")
	if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []int{unix.O_RDONLY, unix.O_WRONLY, unix.O_RDWR, unix.O_PATH} {
		fd, err := unix.Open(path, mode|unix.O_CLOEXEC, 0)
		if err != nil {
			t.Fatal(err)
		}
		accepted := readOnlyInspectionDescriptor(uintptr(fd))
		if err := unix.Close(fd); err != nil {
			t.Fatal(err)
		}
		if accepted != (mode == unix.O_RDONLY) {
			t.Fatal("descriptor authority incorrectly admitted", mode, accepted)
		}
		if readOnlyInspectionDescriptor(uintptr(fd)) {
			t.Fatal("closed descriptor admitted")
		}
	}
}

func TestInspectionPackageRequiresExactPrivateRootInode(t *testing.T) {
	valid := syscall.Stat_t{Uid: 0, Gid: 0, Nlink: 1, Mode: unix.S_IFREG | 0400, Size: 1}
	if !validInspectionPackageStat(&valid) || validInspectionPackageStat(nil) {
		t.Fatal("invalid base inode admission")
	}
	for _, mutate := range []func(*syscall.Stat_t){
		func(s *syscall.Stat_t) { s.Uid = 1 }, func(s *syscall.Stat_t) { s.Gid = 1 },
		func(s *syscall.Stat_t) { s.Nlink = 0 }, func(s *syscall.Stat_t) { s.Nlink = 2 },
		func(s *syscall.Stat_t) { s.Mode |= unix.S_ISUID }, func(s *syscall.Stat_t) { s.Mode |= unix.S_ISGID },
		func(s *syscall.Stat_t) { s.Mode |= unix.S_ISVTX }, func(s *syscall.Stat_t) { s.Mode |= 0040 },
		func(s *syscall.Stat_t) { s.Mode = unix.S_IFIFO | 0400 }, func(s *syscall.Stat_t) { s.Size = 0 },
		func(s *syscall.Stat_t) { s.Size = -1 }, func(s *syscall.Stat_t) { s.Size = (512 << 20) + 1 },
	} {
		altered := valid
		mutate(&altered)
		if validInspectionPackageStat(&altered) {
			t.Fatal("unsafe inherited inode admitted", altered)
		}
	}
	valid.Size = 512 << 20
	if !validInspectionPackageStat(&valid) {
		t.Fatal("maximum bounded package refused")
	}
}
