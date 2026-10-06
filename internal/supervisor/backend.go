package supervisor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"github.com/pranavreddyg17/home-node/internal/hostcheck"
)

type LinuxBackend struct {
	DataRoot    string
	TransferGID int
}
type boundedOutput struct {
	bytes.Buffer
	tooLarge bool
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	n := len(p)
	remaining := (64 << 10) - b.Len()
	if n > remaining {
		b.tooLarge = true
		p = p[:remaining]
	}
	_, _ = b.Buffer.Write(p)
	return n, nil
}
func command(ctx context.Context, input string, name string, args ...string) (string, error) {
	return commandWithFiles(ctx, input, nil, name, args...)
}
func commandWithFiles(ctx context.Context, input string, files []*os.File, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.ExtraFiles = files
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "LC_ALL=C"}
	cmd.Stdin = strings.NewReader(input)
	cmd.WaitDelay = 3 * time.Second
	var out, diagnostic boundedOutput
	cmd.Stdout = &out
	cmd.Stderr = &diagnostic
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s failed: %w", filepath.Base(name), err)
	}
	if out.tooLarge || diagnostic.tooLarge {
		return "", errors.New("command output exceeds limit")
	}
	return out.String(), nil
}
func (b LinuxBackend) ValidateHost(ctx context.Context, p Policy) error {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" || os.Geteuid() != 0 {
		return errors.New("supervisor requires supported Linux host and service identity")
	}
	report := hostcheck.Inspect(b.DataRoot)
	if !report.PrerequisitesMet {
		return errors.New("host prerequisites failed")
	}
	if int64(p.MemoryMiB+2048)*(1<<20) > int64(report.Host.MemoryBytes) || p.VCPUs >= runtime.NumCPU() {
		return ErrCapacity
	}
	caps, err := command(ctx, "", "/usr/bin/virsh", "--connect", "qemu:///system", "capabilities")
	if err != nil {
		return err
	}
	if !strings.Contains(caps, "<model>apparmor</model>") {
		return errors.New("libvirt AppArmor driver unavailable")
	}
	parent := "/sys/fs/cgroup/homenode.slice"
	if err = boundedCgroup(parent, "memory.max", int64(p.MemoryMiB)*(1<<20)); err != nil {
		return err
	}
	if err = boundedCgroup(parent, "pids.max", 512); err != nil {
		return err
	}
	return cpuBound(parent, int64(p.VCPUs))
}
func boundedCgroup(directory, file string, max int64) error {
	data, err := os.ReadFile(filepath.Join(directory, file))
	if err != nil {
		return err
	}
	n, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil || n < 0 || n > max {
		return fmt.Errorf("%s is not bounded by policy", file)
	}
	return nil
}
func cpuBound(directory string, vcpus int64) error {
	data, err := os.ReadFile(filepath.Join(directory, "cpu.max"))
	if err != nil {
		return err
	}
	parts := strings.Fields(string(data))
	if len(parts) != 2 {
		return ErrPolicy
	}
	quota, e1 := strconv.ParseInt(parts[0], 10, 64)
	period, e2 := strconv.ParseInt(parts[1], 10, 64)
	if e1 != nil || e2 != nil || quota <= 0 || period <= 0 || period > 1000000 || quota > vcpus*period {
		return ErrPolicy
	}
	return nil
}
func (b LinuxBackend) Prepare(ctx context.Context, d Domain) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// Reserved DAC domains need coordinated volume and channel ownership.
	// Refuse before shared-account preparation until that lifecycle is wired.
	if d.GuestUID != 0 || d.GuestGID != 0 {
		return ErrPolicy
	}
	if !guestproto.ValidID(d.ID) || filepath.Base(d.DataPath) != d.ID+".raw" || d.DiskReserveBytes < 4<<30 {
		return ErrPolicy
	}
	qemu, err := user.Lookup("libvirt-qemu")
	if err != nil {
		return err
	}
	uid, err := strconv.Atoi(qemu.Uid)
	if err != nil {
		return err
	}
	channelDir := filepath.Dir(d.ChannelPath)
	if err = prepareGuestChannelDirectory(ctx, channelDir, uid, b.TransferGID); err != nil {
		return err
	}
	if err := b.CleanupPreparation(ctx, filepath.Dir(d.DataPath), d.ID, d.Image.DataBytes); err != nil {
		return err
	}
	return prepareDataVolume(ctx, d.DataPath, d.Image.DataBytes, d.DiskReserveBytes)
}
func (b LinuxBackend) Start(ctx context.Context, d Domain) error {
	xml, err := d.XML()
	if err != nil {
		return err
	}
	_, err = command(ctx, xml, "/usr/bin/virsh", "--connect", "qemu:///system", "create", "/dev/stdin")
	return err
}
func (b LinuxBackend) Running(ctx context.Context, id string) (bool, error) {
	if !guestproto.ValidID(id) {
		return false, ErrPolicy
	}
	names, err := command(ctx, "", "/usr/bin/virsh", "--connect", "qemu:///system", "list", "--name")
	if err != nil {
		return false, err
	}
	for _, name := range strings.Fields(names) {
		if name == "homenode-"+id {
			return true, nil
		}
	}
	return false, nil
}
func (b LinuxBackend) Stop(ctx context.Context, id string) error {
	running, err := b.Running(ctx, id)
	if err != nil {
		return err
	}
	if !running {
		return nil
	}
	_, err = command(ctx, "", "/usr/bin/virsh", "--connect", "qemu:///system", "destroy", "homenode-"+id)
	return err
}
func (b LinuxBackend) Verify(ctx context.Context, d Domain) error {
	pidData, err := os.ReadFile(filepath.Join("/run/libvirt/qemu", d.Name()+".pid"))
	if err != nil {
		return err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
	if err != nil || pid < 1 {
		return ErrPolicy
	}
	if d.GuestUID != 0 {
		if err = verifyGuestDACProcess(ctx, pid, d.GuestUID, d.GuestGID); err != nil {
			return err
		}
	}
	proc := filepath.Join("/proc", strconv.Itoa(pid))
	label, err := os.ReadFile(filepath.Join(proc, "attr/current"))
	if err != nil {
		return err
	}
	if !strings.HasPrefix(string(label), "libvirt-") || !strings.HasSuffix(strings.TrimSpace(string(label)), "(enforce)") {
		return errors.New("QEMU process is not confined by enforcing libvirt AppArmor")
	}
	cgroups, err := os.ReadFile(filepath.Join(proc, "cgroup"))
	if err != nil {
		return err
	}
	relative := ""
	for _, line := range strings.Split(string(cgroups), "\n") {
		if value, ok := strings.CutPrefix(line, "0::"); ok {
			relative = value
		}
	}
	if !strings.HasPrefix(relative, "/homenode.slice/") || filepath.Clean(relative) != relative {
		return errors.New("QEMU is outside bounded workload slice")
	}
	if err = boundedCgroup(filepath.Join("/sys/fs/cgroup", relative), "memory.max", int64(d.Image.MemoryMiB+512)*(1<<20)); err != nil {
		return err
	}
	info, err := os.Lstat(d.ChannelPath)
	if err != nil || info.Mode()&os.ModeSocket == 0 {
		return errors.New("guest channel missing")
	}
	if err = os.Chown(d.ChannelPath, -1, b.TransferGID); err != nil {
		return err
	}
	return os.Chmod(d.ChannelPath, 0660)
}
func (b LinuxBackend) FreeBytes(directory string) (int64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(directory, &stat); err != nil {
		return 0, err
	}
	return int64(stat.Bavail) * int64(stat.Bsize), nil
}

func (b LinuxBackend) CleanupPreparation(ctx context.Context, directory, id string, size int64) error {
	return purgeVolumePreparation(ctx, directory, id, size)
}
