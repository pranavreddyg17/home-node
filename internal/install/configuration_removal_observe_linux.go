//go:build linux

package install

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"time"
)

// observeConfigurationRemovalManagerWith is an internal, read-only transition
// observer. Caller retains the installation lock and activation barrier; this
// observation neither stops units nor authorizes publication by itself.
func (e *Engine) observeConfigurationRemovalManagerWith(ctx context.Context, command func(context.Context, string, ...string) *exec.Cmd) error {
	return e.observeConfigurationRemovalManagerStateWith(ctx, command, true)
}

// Present owned fragments may be active before stop. Missing fragments must
// already be dormant, so a partial removal cannot adopt a still-running unit.
func (e *Engine) observeConfigurationRemovalLoadedWith(ctx context.Context, command func(context.Context, string, ...string) *exec.Cmd) error {
	return e.observeConfigurationRemovalManagerStateWith(ctx, command, false)
}
func (e *Engine) observeConfigurationRemovalManagerStateWith(ctx context.Context, command func(context.Context, string, ...string) *exec.Cmd, dormant bool) error {

	if err := ctx.Err(); err != nil {
		return err
	}
	if command == nil {
		return ErrPlan
	}
	installed, err := e.load()
	if err != nil {
		return err
	}
	intent, err := e.loadConfigurationRemovalIntent(installed)
	if err != nil {
		return err
	}
	units, err := configurationRemovalUnits(intent.Plan)
	if err != nil {
		return err
	}
	missing := map[string]bool{}
	records := map[string]record{}
	for _, item := range installed.Items {
		records[item.Path] = item
	}
	observedAbsence := map[string]bool{}
	verify := func() error {
		current, err := e.load()
		if err != nil || !reflect.DeepEqual(current, installed) {
			return ErrConflict
		}
		currentIntent, err := e.loadConfigurationRemovalIntent(current)
		if err != nil || !reflect.DeepEqual(currentIntent, intent) {
			return ErrConflict
		}
		for _, name := range intent.Plan.RetainedPaths {
			if err := e.matches(records[name]); err != nil {
				return err
			}
		}
		for index, file := range intent.Plan.Files {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := e.parents(file.Path); err != nil {
				return err
			}
			_, err := e.host.Lstat(file.Path)
			absent := os.IsNotExist(err)
			if previous, seen := observedAbsence[file.Path]; seen && previous != absent {
				return ErrConflict
			}
			observedAbsence[file.Path] = absent
			if absent {
				if index > intent.Completed {
					return ErrConflict
				}
			} else {
				if err != nil {
					return err
				}
				if index < intent.Completed || intent.Ready {
					return ErrConflict
				}
				if err := e.matches(records[file.Path]); err != nil {
					return err
				}
			}
			if strings.HasPrefix(file.Path, "etc/systemd/system/") {
				missing[strings.TrimPrefix(file.Path, "etc/systemd/system/")] = absent
			}
		}
		return ctx.Err()
	}
	if err := verify(); err != nil {
		return err
	}
	bounded, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	for _, unit := range units {
		step, finish := context.WithTimeout(bounded, 5*time.Second)
		mustBeDormant := dormant || missing[unit]
		properties := "--property=Id,FragmentPath,DropInPaths,NeedDaemonReload,Transient,Job,LoadState"
		if mustBeDormant {
			properties += ",ActiveState,SubState"
		}
		if mustBeDormant && strings.HasSuffix(unit, ".service") {
			properties += ",MainPID,ControlPID"
		}
		cmd := command(step, "/usr/bin/systemctl", "--system", "--no-pager", "--all", "show", properties, unit)
		if cmd == nil {
			finish()
			return ErrPlan
		}
		cmd.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C", "SYSTEMD_COLORS=0", "SYSTEMD_PAGER=cat"}
		output := &recoveryManagerOutput{}
		cmd.Stdout = output
		cmd.Stderr = io.Discard
		cmd.WaitDelay = time.Second
		err := cmd.Run()
		contextErr := step.Err()
		finish()
		if err != nil || contextErr != nil {
			return errors.Join(ErrConflict, err, contextErr)
		}
		if mustBeDormant {
			if err := validateRemovalDormantUnit(output.Bytes(), unit, missing[unit]); err != nil {
				return err
			}
		} else if err := validateRemovalLoadedUnit(output.Bytes(), unit); err != nil {
			return err
		}
		// Loaded application units must still carry the retained marker condition.
		// A genuinely not-found unit was admitted above and has no activation path.
		loaded := strings.Contains("\n"+string(output.Bytes())+"\n", "\nLoadState=loaded\n")
		if loaded && unit != "homenode-inspect.service" && unit != "homenode.slice" {
			if err := observeActivationConditionsForUnitsWith(bounded, command, []string{unit}); err != nil {
				return err
			}
		}

	}
	return verify()
}
