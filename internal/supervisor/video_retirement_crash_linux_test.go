//go:build linux

package supervisor

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/state"
	"golang.org/x/sys/unix"
)

type videoCrashConfiguration struct {
	Request                          Request
	Policy                           Policy
	Pool                             GuestUIDPool
	GuestGID                         uint32
	Images, Volumes, Channels, Point string
}

func videoCrashOptedIn() bool {
	return os.Geteuid() == 0 && os.Getenv("HOMENODE_VIDEO_RETIREMENT_FS_INTEGRATION") == "1" && os.Getenv("HOMENODE_VIDEO_RETIREMENT_HARDLINK_NEGATIVE") == "1"
}

// Only the disposable root fixture invokes this child. Its bounded stdin holds
// generated resource paths and policy, never credentials or user workload data.
func TestNativeVideoRetirementCrashHelper(t *testing.T) {
	if !videoCrashOptedIn() || os.Getenv("HOMENODE_VIDEO_RETIREMENT_CRASH_CHILD") != "1" {
		t.Skip("explicit disposable root crash child required")
	}
	data, err := io.ReadAll(io.LimitReader(os.Stdin, 65537))
	if err != nil || len(data) > 65536 {
		t.Fatal("fixture input")
	}
	var config videoCrashConfiguration
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal("fixture configuration")
	}
	store, err := state.Open(filepath.Join(config.Images, "journal"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	m := &Manager{Store: store, Policy: config.Policy, GuestUIDPool: &config.Pool, GuestGID: config.GuestGID, Backend: LinuxBackend{TransferGID: 64056}, Images: config.Images, Volumes: config.Volumes, Channels: config.Channels}
	ctx := context.Background()
	record, err := m.prepareVideoRetirement(ctx, config.Request, func(context.Context) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	guard := func(context.Context) error {
		var phase string
		if err := store.DB.QueryRow(`SELECT value FROM settings WHERE key=?`, videoRetirementKey(config.Request.InstanceID)+".stage").Scan(&phase); err != nil {
			return err
		}
		match := strings.HasPrefix(config.Point, "phase:") && phase == strings.TrimPrefix(config.Point, "phase:")
		revision := strconv.FormatInt(config.Request.Revision, 10)
		var source, quarantine, pending string
		switch {
		case strings.HasPrefix(config.Point, "socket-"):
			parent := filepath.Join(config.Channels, config.Request.InstanceID)
			source = filepath.Join(parent, "adapter.sock")
			quarantine = filepath.Join(parent, ".adapter-video-retire-"+revision)
			pending = "socket-removing"
		case strings.HasPrefix(config.Point, "volume-"):
			source = filepath.Join(config.Volumes, config.Request.InstanceID+".raw")
			quarantine = filepath.Join(config.Volumes, "."+config.Request.InstanceID+".video-volume-retire-"+revision)
			pending = "volume-removing"
		case strings.HasPrefix(config.Point, "directory-"):
			source = filepath.Join(config.Channels, config.Request.InstanceID)
			quarantine = filepath.Join(config.Channels, "."+config.Request.InstanceID+".video-directory-retire-"+revision)
			pending = "directory-removing"
		}
		if pending != "" && phase == pending {
			_, se := os.Lstat(source)
			_, qe := os.Lstat(quarantine)
			match = strings.HasSuffix(config.Point, "-rename") && errors.Is(se, os.ErrNotExist) && qe == nil || strings.HasSuffix(config.Point, "-unlink") && errors.Is(se, os.ErrNotExist) && errors.Is(qe, os.ErrNotExist)
		}
		if match {
			ready := os.NewFile(3, "fixture-ready")
			if ready == nil {
				return errors.New("fixture readiness descriptor missing")
			}
			if _, err := ready.Write([]byte{1}); err != nil {
				return err
			}
			// SIGKILL bypasses Store.Close and all retained descriptor defers.
			if err := unix.Kill(os.Getpid(), unix.SIGKILL); err != nil {
				return err
			}
			select {}
		}
		return nil
	}
	if _, err := m.retireVideoResources(ctx, record, guard); err != nil {
		t.Fatal(err)
	}
	t.Fatal("fixture crash point was not reached")
}

func TestNativeVideoRetirementCrashRecovery(t *testing.T) {
	if !videoCrashOptedIn() {
		t.Skip("explicit disposable Linux root fixture required")
	}
	points := []string{"phase:socket-removing", "phase:socket-removed", "phase:volume-removing", "phase:volume-removed", "phase:directory-removing", "phase:directory-removed", "socket-rename", "socket-unlink", "volume-rename", "volume-unlink", "directory-rename", "directory-unlink"}
	for _, point := range points {
		t.Run(point, func(t *testing.T) {
			m, record, _, _ := nativeVideoPreparedVolumeFixture(t)
			request := record.Intent.Request
			config := videoCrashConfiguration{Request: request, Policy: m.Policy, Pool: *m.GuestUIDPool, GuestGID: m.GuestGID, Images: m.Images, Volumes: m.Volumes, Channels: m.Channels, Point: point}
			data, err := json.Marshal(config)
			if err != nil {
				t.Fatal(err)
			}
			if err := m.Store.Close(); err != nil {
				t.Fatal(err)
			}
			record = videoRetirementRecord{}
			exe, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			readyRead, readyWrite, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer readyRead.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			child := exec.CommandContext(ctx, exe, "-test.run=^TestNativeVideoRetirementCrashHelper$", "-test.timeout=14s")
			child.Env = []string{"LANG=C", "HOMENODE_VIDEO_RETIREMENT_FS_INTEGRATION=1", "HOMENODE_VIDEO_RETIREMENT_HARDLINK_NEGATIVE=1", "HOMENODE_VIDEO_RETIREMENT_CRASH_CHILD=1"}
			child.Stdin = strings.NewReader(string(data))
			child.Stdout = io.Discard
			child.Stderr = io.Discard
			child.ExtraFiles = []*os.File{readyWrite}
			err = child.Start()
			closeErr := readyWrite.Close()
			if err != nil || closeErr != nil {
				t.Fatal("fixture child start", err, closeErr)
			}
			waitErr := child.Wait()
			var exit *exec.ExitError
			if ctx.Err() != nil || !errors.As(waitErr, &exit) {
				t.Fatal("fixture child did not die at target", waitErr, ctx.Err())
			}
			status, ok := exit.Sys().(syscall.WaitStatus)
			if !ok || !status.Signaled() || status.Signal() != syscall.SIGKILL {
				t.Fatal("unexpected child termination", waitErr)
			}
			ready, err := io.ReadAll(io.LimitReader(readyRead, 2))
			if err != nil || len(ready) != 1 || ready[0] != 1 {
				t.Fatal("crash target not acknowledged", ready, err)
			}
			reopened, err := state.Open(filepath.Join(m.Images, "journal"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = reopened.Close() })
			m = &Manager{Store: reopened, Policy: m.Policy, GuestUIDPool: m.GuestUIDPool, GuestGID: m.GuestGID, Backend: m.Backend, Images: m.Images, Volumes: m.Volumes, Channels: m.Channels}
			record, err = m.prepareVideoRetirement(context.Background(), request, func(context.Context) error { return nil })
			if err != nil {
				t.Fatal("recover killed intent", err)
			}
			got, err := m.retireVideoResources(context.Background(), record, func(context.Context) error { return nil })
			if err != nil || got.Stage != "directory-removed" {
				t.Fatal("recover killed scope", got.Stage, err)
			}
			var leases int
			if err := reopened.DB.QueryRow(`SELECT count(*) FROM runtime_uid_leases WHERE instance_id=?`, request.InstanceID).Scan(&leases); err != nil || leases != 1 {
				t.Fatal("crash recovery released UID", leases, err)
			}
		})
	}
	if !t.Failed() {
		t.Log("reserved video SIGKILL recovery component passed; generated resources and injected stopped guard do not qualify power-loss or live VM cleanup")
	}
}
