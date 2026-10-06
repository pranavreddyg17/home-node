//go:build linux

package supervisor

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestNativeGuestChannelDACIsolation(t *testing.T) {
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_GUEST_UID_ACCOUNTS_INTEGRATION") != "1" {
		t.Skip("explicit disposable Linux root fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := ObserveLocalGuestUIDConflicts(ctx, GuestUIDPool{First: 1000000000, Last: 1000000255})
	if err != nil {
		t.Fatal(err)
	}
	pool, err = ObserveGuestUIDProcessConflicts(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	identities := []uint32{}
	for uid := pool.First; uid <= pool.Last && len(identities) < 3; uid++ {
		if !pool.Blocked[uid] {
			identities = append(identities, uid)
		}
	}
	if len(identities) != 3 {
		t.Fatal("insufficient unoccupied fixture identities")
	}
	guest, sibling, transfer := identities[0], identities[1], identities[2]
	// Only synthetic fixture objects live here; allow traversal to test the
	// object DAC boundaries rather than the test harness's private parent.
	root, err := os.MkdirTemp("/tmp", "homenode-dac-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(root); err != nil {
			t.Errorf("fixture cleanup: %v", err)
		}
	})
	if err := os.Chmod(root, 0755); err != nil {
		t.Fatal(err)
	}
	channels := filepath.Join(root, "channels")
	if err := os.Mkdir(channels, 0755); err != nil {
		t.Fatal(err)
	}
	channel := filepath.Join(channels, "guest")
	const guestGID, transferGID = 64054, 64055
	if err := prepareGuestChannelDirectory(ctx, channel, int(guest), transferGID); err != nil {
		t.Fatal(err)
	}
	socketPath := filepath.Join(channel, "adapter.sock")
	fd, err := unix.Socket(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := unix.Bind(fd, &unix.SockaddrUnix{Name: socketPath}); err != nil {
		unix.Close(fd)
		t.Fatal(err)
	}
	if err := unix.Close(fd); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(socketPath, int(guest), guestGID); err != nil {
		t.Fatal(err)
	}
	if err := grantGuestChannelAccess(ctx, socketPath, guest, transferGID); err != nil {
		t.Fatal(err)
	}
	dataPath := filepath.Join(root, "guest.raw")
	if err := os.WriteFile(dataPath, []byte("synthetic private guest data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(dataPath, int(guest), guestGID); err != nil {
		t.Fatal(err)
	}
	check := func(uid uint32, gid int, path, access string, want bool) {
		t.Helper()
		cmd := exec.CommandContext(ctx, "/usr/bin/setpriv", "--reuid="+strconv.FormatUint(uint64(uid), 10), "--regid="+strconv.Itoa(gid), "--clear-groups", "--inh-caps=-all", "--ambient-caps=-all", "--bounding-set=-all", "--no-new-privs", "/usr/bin/test", access, path)
		cmd.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C"}
		cmd.WaitDelay = time.Second
		err := cmd.Run()
		if want {
			if err != nil {
				t.Fatalf("allowed DAC access refused uid=%d access=%s: %v", uid, access, err)
			}
			return
		}
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 1 {
			t.Fatalf("expected DAC refusal uid=%d access=%s: %v", uid, access, err)
		}
	}
	for _, access := range []string{"-r", "-w"} {
		check(guest, guestGID, dataPath, access, true)
		check(sibling, guestGID, dataPath, access, false)
		check(transfer, transferGID, dataPath, access, false)
		check(guest, guestGID, socketPath, access, true)
		check(sibling, guestGID, socketPath, access, false)
		check(transfer, transferGID, socketPath, access, true)
	}
	if data, err := os.ReadFile(dataPath); err != nil || string(data) != "synthetic private guest data" {
		t.Fatal("permission checks changed data", err)
	}
}
