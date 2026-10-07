//go:build linux

package supervisor

import (
	"bufio"
	"context"
	"errors"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
	"time"
)

// This experiment qualifies a device group only, not libvirt/QEMU launch.
func TestNativeKVMPrimaryGroupAccess(t *testing.T) {
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_KVM_API_INTEGRATION") != "1" {
		t.Skip("explicit disposable Linux KVM experiment")
	}
	info, err := os.Lstat("/dev/kvm")
	if err != nil {
		t.Fatal(err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || info.Mode()&os.ModeCharDevice == 0 || stat.Uid != 0 || stat.Gid == 0 || stat.Gid > 1<<31-1 || info.Mode().Perm() != 0660 || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 || unix.Major(uint64(stat.Rdev)) != 10 || unix.Minor(uint64(stat.Rdev)) != 232 {
		t.Fatal("KVM device is not root-owned with a non-root device group with mode0660")
	}
	if _, err := unix.Lgetxattr("/dev/kvm", "system.posix_acl_access", nil); !errors.Is(err, unix.ENODATA) {
		t.Fatal("ambiguous extended device ACL", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := ObserveLocalGuestUIDConflicts(ctx, GuestUIDPool{First: 2000000000, Last: 2000000255})
	if err != nil {
		t.Fatal(err)
	}
	pool, err = ObserveGuestUIDProcessConflicts(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	var uid uint32
	for candidate := pool.First; candidate <= pool.Last; candidate++ {
		if !pool.Blocked[candidate] {
			uid = candidate
			break
		}
	}
	if uid == 0 {
		t.Fatal("no unused experiment UID")
	}
	// Fixed Python code holds the process after the query for protected procfs
	// credential verification. It creates no VM and writes no host files.
	const script = `import fcntl, os, stat, sys
pinned=os.open('/dev/kvm', os.O_PATH|os.O_NOFOLLOW|os.O_CLOEXEC)
s=os.fstat(pinned)
if not stat.S_ISCHR(s.st_mode) or os.major(s.st_rdev)!=10 or os.minor(s.st_rdev)!=232:
    raise RuntimeError('unexpected device')
fd=os.open('/proc/self/fd/%d' % pinned, os.O_RDWR|os.O_CLOEXEC)
opened=os.fstat(fd)
if (opened.st_dev,opened.st_ino,opened.st_rdev)!=(s.st_dev,s.st_ino,s.st_rdev):
    raise RuntimeError('device identity changed')
if fcntl.ioctl(fd, 0xAE00, 0)!=12:
    raise RuntimeError('unsupported KVM API')
print('KVM_API_READY', flush=True)
sys.stdin.buffer.read(1)
os.close(fd)
os.close(pinned)
`
	cmd := exec.CommandContext(ctx, "/usr/bin/setpriv", "--reuid="+strconv.FormatUint(uint64(uid), 10), "--regid="+strconv.FormatUint(uint64(stat.Gid), 10), "--clear-groups", "--inh-caps=-all", "--ambient-caps=-all", "--bounding-set=-all", "--no-new-privs", "--pdeathsig=SIGKILL", "/usr/bin/python3", "-I", "-B", "-c", script)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C", "PYTHONDONTWRITEBYTECODE=1"}
	cmd.WaitDelay = time.Second
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
	cmd.Stderr = io.Discard
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdout.Close()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cancel()
		_ = stdin.Close()
		err := cmd.Wait()
		var exit *exec.ExitError
		if err != nil && !errors.As(err, &exit) {
			t.Errorf("experiment cleanup: %v", err)
		}
	}()
	ready, err := bufio.NewReader(io.LimitReader(stdout, 64)).ReadString('\n')
	if err != nil || ready != "KVM_API_READY\n" {
		t.Fatal("primary group could not access KVM API", err)
	}
	if err := verifyGuestDACProcess(ctx, cmd.Process.Pid, uid, stat.Gid); err != nil {
		t.Fatal("device query process gained unexpected authority", err)
	}

	wrongGID := uid
	if wrongGID == stat.Gid {
		wrongGID++
	}
	const deniedScript = `import errno, os, stat
pinned=os.open('/dev/kvm', os.O_PATH|os.O_NOFOLLOW|os.O_CLOEXEC)
s=os.fstat(pinned)
if not stat.S_ISCHR(s.st_mode) or os.major(s.st_rdev)!=10 or os.minor(s.st_rdev)!=232:
    raise RuntimeError('unexpected device')
try:
    fd=os.open('/proc/self/fd/%d' % pinned, os.O_RDWR|os.O_CLOEXEC)
except PermissionError as exc:
    if exc.errno!=errno.EACCES:
        raise
    print('KVM_GROUP_DENIED', flush=True)
else:
    os.close(fd)
    raise RuntimeError('unqualified group acquired KVM access')
os.close(pinned)
`
	denied := exec.CommandContext(ctx, "/usr/bin/setpriv", "--reuid="+strconv.FormatUint(uint64(uid), 10), "--regid="+strconv.FormatUint(uint64(wrongGID), 10), "--clear-groups", "--inh-caps=-all", "--ambient-caps=-all", "--bounding-set=-all", "--no-new-privs", "--pdeathsig=SIGKILL", "/usr/bin/python3", "-I", "-B", "-c", deniedScript)
	denied.Env = cmd.Env
	denied.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
	denied.WaitDelay = time.Second
	var denial boundedOutput
	denied.Stdout = &denial
	denied.Stderr = io.Discard
	if err := denied.Run(); err != nil || denial.tooLarge || denial.String() != "KVM_GROUP_DENIED\n" {
		t.Fatal("unqualified primary group did not receive permission denial", err)
	}
	if _, err := unix.Lgetxattr("/dev/kvm", "system.posix_acl_access", nil); !errors.Is(err, unix.ENODATA) {
		t.Fatal("device ACL changed during experiment", err)
	}
	current, err := os.Lstat("/dev/kvm")
	var currentStat *syscall.Stat_t
	currentOK := false
	if current != nil {
		currentStat, currentOK = current.Sys().(*syscall.Stat_t)
	}
	if err != nil || !currentOK || currentStat.Uid != stat.Uid || currentStat.Gid != stat.Gid || currentStat.Rdev != stat.Rdev || !os.SameFile(info, current) || current.Mode() != info.Mode() {
		t.Fatal("device policy changed during experiment", err)
	}
}
