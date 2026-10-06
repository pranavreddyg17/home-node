//go:build linux

package supervisor

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
	"time"
)

func TestNativeGuestDACProcessVerification(t *testing.T) {
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_GUEST_UID_ACCOUNTS_INTEGRATION") != "1" {
		t.Skip("explicit disposable Linux root fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := ObserveLocalGuestUIDConflicts(ctx, GuestUIDPool{First: 1000000000, Last: 1000000255})
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
		t.Fatal("no unoccupied fixture UID")
	}
	identity := strconv.FormatUint(uint64(uid), 10)
	cmd := exec.CommandContext(ctx, "/usr/bin/setpriv", "--reuid="+identity, "--regid="+identity,
		"--clear-groups", "--inh-caps=-all", "--ambient-caps=-all", "--bounding-set=-all",
		"--no-new-privs", "--pdeathsig=SIGKILL", "/bin/sh", "-c", "printf 'ready\n'; exec /bin/sleep 60")
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C"}
	cmd.Stderr = io.Discard
	cmd.WaitDelay = time.Second
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cancel()
		err := cmd.Wait()
		var exit *exec.ExitError
		if err != nil && !errors.As(err, &exit) {
			t.Errorf("fixture cleanup: %v", err)
		}
	}()
	ready, err := bufio.NewReader(io.LimitReader(stdout, 64)).ReadString('\n')
	if err != nil || ready != "ready\n" {
		t.Fatal("credential transition did not signal readiness", ready, err)
	}
	if err = verifyGuestDACProcess(ctx, cmd.Process.Pid, uid, uid); err != nil {
		t.Fatal("matching guest credentials refused", err)
	}
	if err = verifyGuestDACProcess(ctx, cmd.Process.Pid, uid+1, uid); !errors.Is(err, ErrPolicy) {
		t.Fatal("wrong UID accepted", err)
	}
	if err = verifyGuestDACProcess(ctx, cmd.Process.Pid, uid, uid+1); !errors.Is(err, ErrPolicy) {
		t.Fatal("wrong GID accepted", err)
	}
	cancelled, stop := context.WithCancel(ctx)
	stop()
	if err = verifyGuestDACProcess(cancelled, cmd.Process.Pid, uid, uid); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation ignored", err)
	}
}
