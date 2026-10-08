//go:build linux || darwin

package install

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

func TestGuestUIDIntentCrashChild(t *testing.T) {
	phase := os.Getenv("HOMENODE_UID_INTENT_CRASH_PHASE")
	if phase == "" {
		t.Skip("parent-only crash fixture")
	}
	if phase != "guest-uid-intent-created" && phase != "guest-uid-intent-written" && phase != "guest-uid-intent-durable" {
		t.Fatal("invalid crash phase")
	}
	engine, err := Open(os.Getenv("HOMENODE_UID_INTENT_CRASH_HOST"), os.Getenv("HOMENODE_UID_INTENT_CRASH_JOURNAL"))
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	plan, err := guestUIDProvisioningPlan(context.Background(), strings.Repeat("a", 32), supervisor.GuestUIDPool{First: 2000000000, Last: 2000000001}, []uint32{998, 997})
	if err != nil {
		t.Fatal(err)
	}
	engine.checkpoint = func(point, name string) error {
		if point == phase {
			if err := syscall.Kill(os.Getpid(), syscall.SIGKILL); err != nil {
				return err
			}
			// Successful signal submission can return before process termination.
			// Never resume the writer while the kernel delivers SIGKILL.
			for {
				time.Sleep(time.Hour)
			}
		}
		return nil
	}
	if err := engine.commitGuestUIDIntent(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	t.Fatal("crash checkpoint was not reached")
}

func TestGuestUIDIntentRecoversAfterProcessKill(t *testing.T) {
	for _, phase := range []string{"guest-uid-intent-created", "guest-uid-intent-written", "guest-uid-intent-durable"} {
		t.Run(phase, func(t *testing.T) {
			host, jr := roots(t)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestGuestUIDIntentCrashChild$", "-test.count=1")
			child.WaitDelay = time.Second
			child.Env = append(os.Environ(), "HOMENODE_UID_INTENT_CRASH_PHASE="+phase, "HOMENODE_UID_INTENT_CRASH_HOST="+host, "HOMENODE_UID_INTENT_CRASH_JOURNAL="+jr)
			output, err := child.CombinedOutput()
			var killed *exec.ExitError
			if !errors.As(err, &killed) || ctx.Err() != nil {
				t.Fatalf("child did not crash at checkpoint: %v %s", err, output)
			}
			status, ok := killed.Sys().(syscall.WaitStatus)
			if !ok || !status.Signaled() || status.Signal() != syscall.SIGKILL {
				t.Fatalf("unexpected child termination: %v %s", err, output)
			}
			path := filepath.Join(jr, "guest-uid-intent.json")
			before, err := os.Stat(path)
			if err != nil {
				t.Fatal("crashed intent disappeared", err)
			}
			contents, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if phase == "guest-uid-intent-created" && len(contents) != 0 {
				t.Fatal("writer advanced beyond creation crash checkpoint")
			}
			// Reopening also proves the kernel released the dead installer's flock.
			engine := openEngine(t, host, jr)
			defer engine.Close()
			plan, err := guestUIDProvisioningPlan(context.Background(), strings.Repeat("a", 32), supervisor.GuestUIDPool{First: 2000000000, Last: 2000000001}, []uint32{998, 997})
			if err != nil {
				t.Fatal(err)
			}
			err = engine.commitGuestUIDIntent(context.Background(), plan)
			if phase == "guest-uid-intent-created" {
				if err == nil {
					t.Fatal("empty crashed intent adopted", err)
				}
			} else if err != nil {
				t.Fatal("complete crashed intent could not retry", err)
			}
			after, err := os.Stat(path)
			if err != nil || !os.SameFile(before, after) {
				t.Fatal("crash retry replaced intent", err)
			}
			preserved, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(contents, preserved) {
				t.Fatal("crash retry changed intent bytes", err)
			}
		})
	}
}
