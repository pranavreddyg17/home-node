//go:build linux

package install

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"strings"
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

func TestRecoveryQuiescenceRefusalsPrecedeStop(t *testing.T) {
	for _, fault := range []string{"foreign-fragment", "missing-guard", "missing-marker", "stop-failed", "guest-remains", "none"} {
		t.Run(fault, func(t *testing.T) {
			stops, guestChecks, markerChecks := 0, 0, 0
			command := func(ctx context.Context, path string, args ...string) *exec.Cmd {
				response, status := "", "0"
				switch {
				case path == "/usr/bin/busctl":
					response = `{"type":"a(sbbsi)","data":[["ConditionPathExists",false,true,"/var/lib/homenode-install/recovery-blocked",0]]}`
					if fault == "missing-guard" {
						response = `{"type":"a(sbbsi)","data":[]}`
					}
				case path == "/usr/bin/systemctl" && len(args) > 3 && args[3] == "stop":
					stops++
					if fault == "stop-failed" {
						status = "1"
					}
				case path == "/usr/bin/systemctl" && len(args) == 6 && args[3] == "show":
					unit := args[5]
					fragment := "/etc/systemd/system/" + unit
					if fault == "foreign-fragment" {
						fragment = "/run/systemd/system/" + unit
					}
					response = "Id=" + unit + "\nFragmentPath=" + fragment + "\nDropInPaths=\nNeedDaemonReload=no\nTransient=no\nJob=\nLoadState=loaded\n"
					if strings.Contains(args[4], "ActiveState") {
						response += "ActiveState=inactive\nSubState=dead\n"
						if unit != "homenode-backup-credential.socket" {
							response += "MainPID=0\nControlPID=0\n"
						}
					}
				default:
					t.Fatal("unexpected command", path, args)
				}
				return exec.CommandContext(ctx, "/bin/sh", "-c", `printf '%s' "$1"; exit "$2"`, "fixture", response, status)
			}
			marker := func(context.Context) error {
				markerChecks++
				if fault == "missing-marker" {
					return ErrConflict
				}
				return nil
			}
			guests := func(context.Context) error {
				guestChecks++
				if fault == "guest-remains" {
					return ErrConflict
				}
				return nil
			}
			err := quiesceRecoveryManagerWith(context.Background(), command, marker, guests)
			if fault == "none" {
				if err != nil || stops != 1 || guestChecks != 1 || markerChecks != 2 {
					t.Fatal("guarded stop incomplete", stops, guestChecks, markerChecks, err)
				}
			} else if !errors.Is(err, ErrConflict) {
				t.Fatal("unsafe quiescence admitted", err)
			}
			if (fault == "foreign-fragment" || fault == "missing-guard" || fault == "missing-marker") && stops != 0 {
				t.Fatal("stop preceded authority refusal", stops)
			}
			if fault == "stop-failed" && guestChecks != 0 {
				t.Fatal("failed stop continued", guestChecks)
			}
		})
	}
}
