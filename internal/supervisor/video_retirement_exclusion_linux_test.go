//go:build linux

package supervisor

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestNativeVideoRetirementStartExclusion(t *testing.T) {
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_VIDEO_RETIREMENT_FS_INTEGRATION") != "1" {
		t.Skip("explicit disposable Linux root fixture required")
	}
	t.Run("retains-start-lock-and-releases-on-error", func(t *testing.T) {
		m, r := retirementSnapshotFixture(t)
		lost := errors.New("fixture callback interruption")
		err := m.withVideoRetirementStartExclusionObserved(context.Background(), r, func(context.Context, string) (bool, error) { return false, nil }, func(snapshot videoRetirementSnapshot, guard func(context.Context) error) error {
			if snapshot.InstanceID != r.InstanceID {
				t.Fatal("wrong snapshot")
			}
			waiter, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			if unlock, err := m.lockStart(waiter); !errors.Is(err, context.DeadlineExceeded) {
				if unlock != nil {
					unlock()
				}
				t.Fatal("start exclusion lost", err)
			}
			if err := guard(context.Background()); err != nil {
				return err
			}
			return lost
		})
		if !errors.Is(err, lost) {
			t.Fatal("callback failure lost", err)
		}
		if !m.startMu.TryLock() {
			t.Fatal("start lock leaked")
		}
		m.startMu.Unlock()
	})
	t.Run("observes-new-reserved-uid-process", func(t *testing.T) {
		m, r := retirementSnapshotFixture(t)
		err := m.withVideoRetirementStartExclusionObserved(context.Background(), r, func(context.Context, string) (bool, error) { return false, nil }, func(snapshot videoRetirementSnapshot, guard func(context.Context) error) error {
			cmd := exec.Command("/bin/sleep", "30")
			cmd.Env = []string{"LANG=C"}
			cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: snapshot.Channel.UID, Gid: snapshot.Channel.GuestGID, NoSetGroups: true}}
			if err := cmd.Start(); err != nil {
				return err
			}
			defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
			if err := guard(context.Background()); !errors.Is(err, ErrPolicy) {
				t.Fatal("reserved UID process admitted", err)
			}
			return errors.New("fixture child remains until deferred reap")
		})
		if err == nil {
			t.Fatal("process refusal lost")
		}
	})
	t.Run("refuses-running-backend", func(t *testing.T) {
		m, r := retirementSnapshotFixture(t)

		called := false
		err := m.withVideoRetirementStartExclusionObserved(context.Background(), r, func(context.Context, string) (bool, error) { return true, nil }, func(videoRetirementSnapshot, func(context.Context) error) error { called = true; return nil })
		if !errors.Is(err, ErrPolicy) || called {
			t.Fatal("running backend admitted", called, err)
		}
	})
	t.Run("rechecks-ownership-after-callback", func(t *testing.T) {
		m, r := retirementSnapshotFixture(t)
		err := m.withVideoRetirementStartExclusionObserved(context.Background(), r, func(context.Context, string) (bool, error) { return false, nil }, func(videoRetirementSnapshot, func(context.Context) error) error {
			_, err := m.Store.DB.Exec(`UPDATE runtime_volume_ownership SET inode=101 WHERE instance_id=?`, r.InstanceID)
			return err
		})
		if !errors.Is(err, ErrPolicy) {
			t.Fatal("changed provenance admitted after callback", err)
		}
	})
	if !t.Failed() {
		t.Log("reserved video start exclusion component passed; backend absence is injected; real process observations do not qualify escaped-fork exclusion or UID release")
	}
}
