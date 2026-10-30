//go:build linux

package supervisor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
)

type qemuDescriptorMetadata struct {
	FD                   int    `json:"fd"`
	Kind                 string `json:"kind"`
	Target               string `json:"target"`
	Flags                string `json:"flags"`
	UID, GID             uint32
	Device, Inode, Links uint64
	Major, Minor, Mode   uint32
	Role                 string              `json:"role"`
	NamedScope           string              `json:"namedScope,omitempty"`
	Socket               *qemuSocketMetadata `json:"socket,omitempty"`
}

func sameQEMUDescriptorStat(a, b unix.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.Mode == b.Mode && a.Uid == b.Uid && a.Gid == b.Gid && a.Nlink == b.Nlink && a.Rdev == b.Rdev
}
func qemuRegularStat(s unix.Stat_t) qemuRegularIdentity {
	return qemuRegularIdentity{Device: uint64(s.Dev), Inode: s.Ino, Links: s.Nlink, UID: s.Uid, GID: s.Gid, Mode: s.Mode}
}
func readQEMUProcFile(directory int, name string, limit int64) ([]byte, error) {
	// The caller authenticates and pins the process directory. Keep this
	// primitive metadata-only even if a later caller supplies a bad name/bound.
	maximum := int64(4096)
	if name != "attr/current" {
		if !strings.HasPrefix(name, "fdinfo/") {
			return nil, ErrPolicy
		}
		slot := strings.TrimPrefix(name, "fdinfo/")
		number, err := strconv.ParseUint(slot, 10, 32)
		if err != nil || number > 1<<20 || strconv.FormatUint(number, 10) != slot {
			return nil, ErrPolicy
		}
		maximum = 8192
	}
	if limit < 1 || limit > maximum {
		return nil, ErrPolicy
	}
	fd, err := unix.Openat(directory, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), "qemu-proc-metadata")
	data, readErr := io.ReadAll(io.LimitReader(file, limit+1))
	if err := errors.Join(readErr, file.Close()); err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, ErrPolicy
	}
	return data, nil
}

// Bounded observation of the candidate static-profile launch shape, not an
// admission receipt. Never reads target data or opens devices for I/O. Socket
// duplicates are metadata-only and close within their helper. No production
// enablement or UID release follows from a successful observation.
func observeQEMUDescriptorMetadata(ctx context.Context, d Domain, pid int) (records []qemuDescriptorMetadata, result error) {
	if os.Geteuid() != 0 || !d.ConfinedWrites || !guestproto.ValidID(d.ID) || d.Image.VCPUs < 1 || d.Image.VCPUs > 256 {
		return nil, ErrPolicy
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := verifyGuestDACProcess(ctx, pid, d.GuestUID, d.GuestGID); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot("/proc")
	if err != nil {
		return nil, err
	}
	defer func() { result = errors.Join(result, root.Close()) }()
	if err := qualifyGuestUIDNamespace(root); err != nil {
		return nil, err
	}
	pidfd, err := unix.PidfdOpen(pid, 0)
	if err != nil {
		return nil, err
	}
	defer func() { result = errors.Join(result, unix.Close(pidfd)) }()
	live := func() error {
		events := []unix.PollFd{{Fd: int32(pidfd), Events: unix.POLLIN}}
		n, err := unix.Poll(events, 0)
		if err != nil {
			return err
		}
		if n != 0 {
			return ErrPolicy
		}
		return ctx.Err()
	}
	if err := live(); err != nil {
		return nil, err
	}
	process, err := root.OpenFile(strconv.Itoa(pid), os.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	defer func() { result = errors.Join(result, process.Close()) }()
	var filesystem unix.Statfs_t
	if err := unix.Fstatfs(int(process.Fd()), &filesystem); err != nil {
		return nil, err
	}
	if filesystem.Type != unix.PROC_SUPER_MAGIC {
		return nil, ErrPolicy
	}
	checkLabel := func() error {
		label, err := readQEMUProcFile(int(process.Fd()), "attr/current", 4096)
		if err != nil {
			return err
		}
		if strings.TrimSpace(string(label)) != d.AppArmorName()+" (enforce)" {
			return ErrPolicy
		}
		return nil
	}
	if err := checkLabel(); err != nil {
		return nil, err
	}
	runtimeID := func() (uint32, error) {
		output, err := command(ctx, "", "/usr/bin/virsh", "--connect", "qemu:///system", "domid", d.Name())
		if err != nil {
			return 0, err
		}
		n, err := strconv.ParseUint(strings.TrimSpace(output), 10, 31)
		if err != nil || n == 0 {
			return 0, ErrPolicy
		}
		return uint32(n), nil
	}
	admittedRuntimeID, err := runtimeID()
	if err != nil {
		return nil, err
	}
	directory, err := unix.Openat(int(process.Fd()), "fd", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	files := os.NewFile(uintptr(directory), "qemu-descriptor-directory")
	defer func() { result = errors.Join(result, files.Close()) }()
	entries, err := files.ReadDir(257)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(entries) > 256 {
		return nil, ErrPolicy
	}
	pipes := map[int]qemuOutputPipe{}
	sockets := map[string]int{}
	records = make([]qemuDescriptorMetadata, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		r, err := readQEMUDescriptorMetadata(d, admittedRuntimeID, int(process.Fd()), directory, pidfd, entry.Name())
		if err != nil {
			return nil, fmt.Errorf("observe QEMU descriptor %s: %w", entry.Name(), err)
		}
		if r.Kind == "pipe" {
			flags, err := strconv.ParseUint(r.Flags, 8, 32)
			if err != nil {
				return nil, ErrPolicy
			}
			pipes[r.FD] = qemuOutputPipe{FD: r.FD, Target: r.Target, Identity: qemuRegularIdentity{Device: r.Device, Inode: r.Inode, Links: r.Links, UID: r.UID, GID: r.GID, Mode: r.Mode}, Access: uint32(flags) & unix.O_ACCMODE}
		}
		if r.Kind == "socket" {
			sockets[r.Role]++
		}
		records = append(records, r)
	}
	if len(pipes) != 2 || qualifyQEMUOutputPipes(pipes[1], pipes[2]) != nil {
		return nil, ErrPolicy
	}
	if len(sockets) != 3 || sockets["candidate-adapter-listener"] != 1 || sockets["candidate-monitor-listener"] != 1 || sockets["candidate-monitor-connected"] != 1 {
		return nil, ErrPolicy
	}
	currentRuntimeID, err := runtimeID()
	if err != nil {
		return nil, err
	}
	if currentRuntimeID != admittedRuntimeID {
		return nil, ErrPolicy
	}
	if err := checkLabel(); err != nil {
		return nil, err
	}
	if err := verifyGuestDACProcess(ctx, pid, d.GuestUID, d.GuestGID); err != nil {
		return nil, err
	}
	if err := live(); err != nil {
		return nil, err
	}
	return records, nil
}

func readQEMUDescriptorMetadata(d Domain, runtimeID uint32, process, directory, pidfd int, name string) (qemuDescriptorMetadata, error) {
	var empty qemuDescriptorMetadata
	number, err := strconv.Atoi(name)
	if err != nil || number < 0 || number > 1<<20 || strconv.Itoa(number) != name {
		return empty, ErrPolicy
	}
	flags := func() (uint32, error) {
		data, err := readQEMUProcFile(process, "fdinfo/"+name, 8192)
		if err != nil {
			return 0, err
		}
		return parseQEMUFDFlags(data)
	}
	target := func() (string, error) {
		buffer := make([]byte, 1025)
		n, err := unix.Readlinkat(directory, name, buffer)
		if err != nil {
			return "", err
		}
		if n > 1024 {
			return "", ErrPolicy
		}
		return string(buffer[:n]), nil
	}
	firstFlags, err := flags()
	if err != nil {
		return empty, err
	}
	firstTarget, err := target()
	if err != nil {
		return empty, err
	}
	var first unix.Stat_t
	// Intentionally follows only this procfs magic link for metadata, never I/O.
	if err := unix.Fstatat(directory, name, &first, 0); err != nil {
		return empty, err
	}
	stable := func() error {
		currentFlags, err := flags()
		if err != nil {
			return err
		}
		currentTarget, err := target()
		if err != nil {
			return err
		}
		var current unix.Stat_t
		if err := unix.Fstatat(directory, name, &current, 0); err != nil {
			return err
		}
		if currentFlags != firstFlags || currentTarget != firstTarget || !sameQEMUDescriptorStat(first, current) {
			return ErrPolicy
		}
		return nil
	}
	if err := stable(); err != nil {
		return empty, err
	}
	access := firstFlags & unix.O_ACCMODE
	r := qemuDescriptorMetadata{FD: number, Target: firstTarget, Flags: strconv.FormatUint(uint64(firstFlags), 8), UID: first.Uid, GID: first.Gid, Device: uint64(first.Dev), Inode: first.Ino, Links: first.Nlink, Mode: first.Mode}
	identity := qemuRegularStat(first)
	switch first.Mode & unix.S_IFMT {
	case unix.S_IFREG:
		r.Kind = "regular"
		if firstTarget != d.DataPath && firstTarget != d.SystemPath && firstTarget != filepath.Join("/run/libvirt/qemu", d.Name()+".pid") {
			return empty, ErrPolicy
		}
		var named unix.Stat_t
		if err := unix.Lstat(firstTarget, &named); err != nil {
			return empty, err
		}
		qualify := qualifyQEMURegularWrite
		if access == unix.O_RDONLY {
			qualify = qualifyQEMURegularRead
		}
		r.Role, err = qualify(d, firstTarget, identity, qemuRegularStat(named), access)
	case unix.S_IFCHR:
		r.Kind = "character-device"
		r.Major, r.Minor = unix.Major(uint64(first.Rdev)), unix.Minor(uint64(first.Rdev))
		if firstTarget != "/dev/null" && firstTarget != "/dev/urandom" && firstTarget != "/dev/kvm" {
			return empty, ErrPolicy
		}
		deviceIdentity := func(s unix.Stat_t) qemuDeviceIdentity {
			return qemuDeviceIdentity{qemuRegularIdentity: qemuRegularStat(s), Major: unix.Major(uint64(s.Rdev)), Minor: unix.Minor(uint64(s.Rdev))}
		}
		var named unix.Stat_t
		if err := unix.Lstat(firstTarget, &named); err != nil {
			return empty, err
		}
		r.Role, err = qualifyQEMUDevice(d, firstTarget, deviceIdentity(first), deviceIdentity(named), access)
		r.NamedScope = "observer-root"
		if err != nil {
			if err := unix.Fstatat(process, "root"+firstTarget, &named, unix.AT_SYMLINK_NOFOLLOW); err != nil {
				return empty, err
			}
			r.Role, err = qualifyQEMUDevice(d, firstTarget, deviceIdentity(first), deviceIdentity(named), access)
			r.NamedScope = "process-root"
		}
	case unix.S_IFSOCK:
		r.Kind = "socket"
		metadata, observeErr := observeQEMUSocketMetadata(pidfd, number, first)
		if observeErr != nil {
			return empty, observeErr
		}
		s := qemuSocketIdentity{qemuRegularIdentity: identity, Family: metadata.Domain, Type: metadata.Type, Protocol: metadata.Protocol, Listening: metadata.Listening, Local: metadata.Local, Peer: metadata.Peer}
		if metadata.PeerCredentials != nil {
			s.PeerPID = metadata.PeerCredentials.Pid
			s.PeerUID = metadata.PeerCredentials.Uid
			s.PeerGID = metadata.PeerCredentials.Gid
		}
		r.Role, err = qualifyQEMUSocket(d, runtimeID, s, access)
		r.Socket = &metadata
	case unix.S_IFIFO:
		r.Kind = "pipe"
		if (number != 1 && number != 2) || access != unix.O_WRONLY {
			return empty, ErrPolicy
		}
		r.Role = "candidate-log-stdout"
		if number == 2 {
			r.Role = "candidate-log-stderr"
		}
	default:
		r.Kind = "other"
		r.Role, err = qualifyQEMUAnonymous(d, firstTarget, identity, access)
	}
	if err != nil {
		return empty, err
	}
	if r.Role == "" {
		return empty, ErrPolicy
	}
	if err := stable(); err != nil {
		return empty, err
	}
	return r, nil
}
