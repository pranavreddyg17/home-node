//go:build linux

package supervisor

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

// Metadata-only observation: never opens a QEMU descriptor for data I/O. Procfs
// magic links are followed only by stat, intentionally, after process admission.
func nativeQEMUDescriptorInventory(t *testing.T, ctx context.Context, d Domain, pid int) {
	t.Helper()
	if os.Getenv("HOMENODE_CONFINEMENT_INVENTORY") != "1" {
		return
	}
	if err := verifyGuestDACProcess(ctx, pid, d.GuestUID, d.GuestGID); err != nil {
		t.Fatal(err)
	}
	pidfd, err := unix.PidfdOpen(pid, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(pidfd)
	live := func() {
		t.Helper()
		fds := []unix.PollFd{{Fd: int32(pidfd), Events: unix.POLLIN}}
		if n, err := unix.Poll(fds, 0); err != nil || n != 0 {
			t.Fatal("descriptor inventory process exited", err)
		}
	}
	live()
	runtimeID := func() uint32 {
		t.Helper()
		output, err := command(ctx, "", "/usr/bin/virsh", "--connect", "qemu:///system", "domid", d.Name())
		value, parseErr := strconv.ParseUint(strings.TrimSpace(output), 10, 31)
		if err != nil || parseErr != nil || value == 0 {
			t.Fatal("live domain numeric identity unavailable", err, parseErr)
		}
		return uint32(value)
	}
	admittedRuntimeID := runtimeID()
	process, err := os.OpenFile(filepath.Join("/proc", strconv.Itoa(pid)), os.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer process.Close()
	fdDirectory, err := unix.Openat(int(process.Fd()), "fd", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	files := os.NewFile(uintptr(fdDirectory), "process-descriptors")
	defer files.Close()
	entries, err := files.ReadDir(257)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
	if len(entries) > 256 {
		t.Fatal("descriptor inventory exceeds bound")
	}
	flags := func(name string) int {
		t.Helper()
		fd, err := unix.Openat(int(process.Fd()), "fdinfo/"+name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			t.Fatal(err)
		}
		file := os.NewFile(uintptr(fd), "descriptor-info")
		data, readErr := io.ReadAll(io.LimitReader(file, 8193))
		closeErr := file.Close()
		if readErr != nil || closeErr != nil || len(data) > 8192 {
			t.Fatal("descriptor flags bound", readErr, closeErr)
		}
		found := false
		value := 0
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "flags:") {
				if found {
					t.Fatal("duplicate flags")
				}
				found = true
				n, err := strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(line, "flags:")), 8, 32)
				if err != nil {
					t.Fatal(err)
				}
				value = int(n)
			}
		}
		if !found {
			t.Fatal("descriptor flags missing")
		}
		return value
	}
	target := func(name string) string {
		t.Helper()
		buffer := make([]byte, 1025)
		n, err := unix.Readlinkat(int(files.Fd()), name, buffer)
		if err != nil || n > 1024 {
			t.Fatal("descriptor target bound", err)
		}
		return string(buffer[:n])
	}
	type descriptor struct {
		FD                   int    `json:"fd"`
		Kind                 string `json:"kind"`
		Target               string `json:"target"`
		Flags                string `json:"flags"`
		UID, GID             uint32
		Device, Inode, Links uint64
		Major, Minor         uint32
		Mode                 uint32
		Role                 string                    `json:"role"`
		NamedScope           string                    `json:"namedScope,omitempty"`
		Socket               *nativeQEMUSocketMetadata `json:"socket,omitempty"`
	}
	records := make([]descriptor, 0, len(entries))
	writableRegular := 0
	socketCount := 0
	socketRoles := map[string]int{}
	for _, entry := range entries {
		name := entry.Name()
		number, err := strconv.Atoi(name)
		if err != nil || number < 0 || number > 1<<20 || strconv.Itoa(number) != name {
			t.Fatal("unqualified descriptor number")
		}
		firstFlags, firstTarget := flags(name), target(name)
		var first, last unix.Stat_t
		if err := unix.Fstatat(int(files.Fd()), name, &first, 0); err != nil {
			t.Fatal(err)
		}
		secondFlags, secondTarget := flags(name), target(name)
		if err := unix.Fstatat(int(files.Fd()), name, &last, 0); err != nil || firstFlags != secondFlags || firstTarget != secondTarget || first.Dev != last.Dev || first.Ino != last.Ino || first.Mode != last.Mode || first.Uid != last.Uid || first.Gid != last.Gid || first.Nlink != last.Nlink || first.Rdev != last.Rdev {
			t.Fatal("descriptor changed during observation", err)
		}
		r := descriptor{FD: number, Target: firstTarget, Flags: strconv.FormatInt(int64(firstFlags), 8), UID: first.Uid, GID: first.Gid, Device: uint64(first.Dev), Inode: first.Ino, Links: first.Nlink, Mode: first.Mode, Role: "unclassified"}
		switch first.Mode & unix.S_IFMT {
		case unix.S_IFREG:
			r.Kind = "regular"
		case unix.S_IFCHR:
			r.Kind = "character-device"
			r.Major, r.Minor = unix.Major(uint64(first.Rdev)), unix.Minor(uint64(first.Rdev))
		case unix.S_IFSOCK:
			r.Kind = "socket"
		case unix.S_IFIFO:
			r.Kind = "pipe"
		default:
			r.Kind = "other"
		}
		if r.Kind == "regular" {
			if firstFlags&unix.O_ACCMODE != unix.O_RDONLY {
				writableRegular++
			}
			var named unix.Stat_t
			if err := unix.Lstat(firstTarget, &named); err != nil {
				t.Fatal("regular descriptor name unavailable", err)
			}
			identity := func(s unix.Stat_t) qemuRegularIdentity {
				return qemuRegularIdentity{Device: uint64(s.Dev), Inode: s.Ino, Links: s.Nlink, UID: s.Uid, GID: s.Gid, Mode: s.Mode}
			}
			qualify := qualifyQEMURegularWrite
			if firstFlags&unix.O_ACCMODE == unix.O_RDONLY {
				qualify = qualifyQEMURegularRead
			}
			role, err := qualify(d, firstTarget, identity(first), identity(named), uint32(firstFlags&unix.O_ACCMODE))
			if err != nil {
				t.Fatal("unexplained regular descriptor", firstTarget, err)
			}
			r.Role = role
		}
		if r.Kind == "character-device" {
			var named unix.Stat_t
			if err := unix.Lstat(firstTarget, &named); err != nil {
				t.Fatal("device name unavailable", err)
			}
			identity := func(s unix.Stat_t) qemuDeviceIdentity {
				return qemuDeviceIdentity{qemuRegularIdentity: qemuRegularIdentity{Device: uint64(s.Dev), Inode: s.Ino, Links: s.Nlink, UID: s.Uid, GID: s.Gid, Mode: s.Mode}, Major: unix.Major(uint64(s.Rdev)), Minor: unix.Minor(uint64(s.Rdev))}
			}
			role, err := qualifyQEMUDevice(d, firstTarget, identity(first), identity(named), uint32(firstFlags&unix.O_ACCMODE))
			r.NamedScope = "observer-root"
			if err != nil {
				// Libvirt may open devices after entering its private mount
				// namespace. Stat through the pinned live process root only;
				// never open the device for I/O or adopt a different node.
				if firstTarget != "/dev/null" && firstTarget != "/dev/urandom" && firstTarget != "/dev/kvm" {
					t.Fatal("unexplained device path")
				}
				if statErr := unix.Fstatat(int(process.Fd()), "root"+firstTarget, &named, unix.AT_SYMLINK_NOFOLLOW); statErr != nil {
					t.Fatal("process device name unavailable", statErr)
				}
				role, err = qualifyQEMUDevice(d, firstTarget, identity(first), identity(named), uint32(firstFlags&unix.O_ACCMODE))
				r.NamedScope = "process-root"
			}
			if err != nil {
				t.Fatal("unexplained device descriptor", firstTarget, err)
			}
			r.Role = role
		}
		if r.Kind == "socket" {
			metadata := nativeQEMUSocketInventory(t, pidfd, number, first)
			socketIdentity := qemuSocketIdentity{qemuRegularIdentity: qemuRegularIdentity{Device: uint64(first.Dev), Inode: first.Ino, Links: first.Nlink, UID: first.Uid, GID: first.Gid, Mode: first.Mode}, Family: metadata.Domain, Type: metadata.Type, Protocol: metadata.Protocol, Listening: metadata.Listening, Local: metadata.Local, Peer: metadata.Peer}
			if metadata.PeerCredentials != nil {
				socketIdentity.PeerPID = metadata.PeerCredentials.Pid
				socketIdentity.PeerUID = metadata.PeerCredentials.Uid
				socketIdentity.PeerGID = metadata.PeerCredentials.Gid
			}
			role, err := qualifyQEMUSocket(d, admittedRuntimeID, socketIdentity, uint32(firstFlags&unix.O_ACCMODE))
			if err != nil {
				t.Fatal("unexplained socket metadata", err)
			}
			r.Role = role
			socketRoles[role]++
			r.Socket = &metadata
			socketCount++
			var current unix.Stat_t
			if err := unix.Fstatat(int(files.Fd()), name, &current, 0); err != nil || current.Dev != first.Dev || current.Ino != first.Ino || current.Mode != first.Mode || current.Uid != first.Uid || current.Gid != first.Gid || current.Nlink != first.Nlink {
				t.Fatal("socket source slot changed", err)
			}
		}
		records = append(records, r)
	}
	live()
	if err := verifyGuestDACProcess(ctx, pid, d.GuestUID, d.GuestGID); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(records)
	if err != nil || len(data) > 128<<10 {
		t.Fatal("descriptor inventory serialization", err)
	}
	if runtimeID() != admittedRuntimeID {
		t.Fatal("live domain numeric identity changed")
	}
	if socketCount != 3 || socketRoles["candidate-adapter-listener"] != 1 || socketRoles["candidate-monitor-listener"] != 1 || socketRoles["candidate-monitor-connected"] != 1 {
		t.Fatal("socket role inventory differs", socketRoles)
	}
	t.Log("native QEMU socket metadata observation completed", "count", socketCount, "admissionQualified", false)
	t.Log("native QEMU descriptor metadata", string(data))
	t.Log("native QEMU descriptor inventory completed", "count", len(records), "writableRegular", writableRegular, "admissionQualified", false)
}
