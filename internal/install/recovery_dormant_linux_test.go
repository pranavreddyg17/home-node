//go:build linux

package install

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

func TestRecoveryDormantQueriesOnlyFixedServices(t *testing.T) {
	valid := "LoadState=loaded\nActiveState=inactive\nSubState=dead\nMainPID=0\nControlPID=0\n"
	units := []string{}
	command := func(ctx context.Context, path string, args ...string) *exec.Cmd {
		if path != "/usr/bin/systemctl" || len(args) != 5 || !reflect.DeepEqual(args[:4], []string{"--system", "--no-pager", "show", "--property=Id,FragmentPath,DropInPaths,NeedDaemonReload,Transient,LoadState,ActiveState,SubState,MainPID,ControlPID"}) {
			t.Fatal("unexpected manager query", path, args)
		}
		units = append(units, args[len(args)-1])
		return exec.CommandContext(ctx, "/bin/sh", "-c", "printf '%s' \"$1\"", "fixture", "Id="+args[4]+"\nFragmentPath=/etc/systemd/system/"+args[4]+"\nDropInPaths=\nNeedDaemonReload=no\nTransient=no\n"+valid)
	}
	if err := observeRecoveryServicesWith(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(units, []string{"homenode-control.service", "homenode-transfer.service", "homenode-supervisor.service", "homenode-backup.service"}) {
		t.Fatal("service set", units)
	}
	valid = strings.Replace(valid, "MainPID=0", "MainPID=1", 1)
	units = nil
	if err := observeRecoveryServicesWith(context.Background(), command); !errors.Is(err, ErrConflict) || len(units) != 1 {
		t.Fatal("unsafe observation continued", units, err)
	}
	var output recoveryManagerOutput
	if _, err := output.Write(make([]byte, 1025)); !errors.Is(err, ErrConflict) || output.Len() != 0 {
		t.Fatal("unbounded manager output", err)
	}
}
