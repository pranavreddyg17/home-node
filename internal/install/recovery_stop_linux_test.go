//go:build linux

package install

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"testing"
	"time"
)

func TestRecoveryStopIsSynchronousFixedAndBounded(t *testing.T) {
	queries := 0
	command := func(ctx context.Context, path string, args ...string) *exec.Cmd {
		queries++
		want := []string{"--system", "--no-pager", "--no-ask-password", "stop", "homenode-backup-credential.socket", "homenode-control.service", "homenode-transfer.service", "homenode-backup.service", "homenode-supervisor.service"}
		if path != "/usr/bin/systemctl" || !reflect.DeepEqual(args, want) {
			t.Fatal("unexpected stop operation", path, args)
		}
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 60*time.Second {
			t.Fatal("unbounded stop")
		}
		return exec.CommandContext(ctx, "/bin/sh", "-c", "exit 0")
	}
	if err := stopRecoveryServicesWith(context.Background(), command); err != nil || queries != 1 {
		t.Fatal("fixed synchronous stop", queries, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := stopRecoveryServicesWith(ctx, command); !errors.Is(err, context.Canceled) || queries != 1 {
		t.Fatal("cancelled stop invoked manager", queries, err)
	}
	if err := stopRecoveryServicesWith(context.Background(), nil); !errors.Is(err, ErrPlan) {
		t.Fatal("missing adapter admitted", err)
	}
	failure := func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "/bin/sh", "-c", "exit 1")
	}
	if err := stopRecoveryServicesWith(context.Background(), failure); !errors.Is(err, ErrConflict) {
		t.Fatal("failed stop admitted", err)
	}
}
