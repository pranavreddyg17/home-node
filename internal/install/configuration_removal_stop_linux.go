//go:build linux

package install

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"time"
)

// stopConfigurationRemovalUnitsWith is only the bounded manager operation.
// Caller holds installation exclusion; guard verifies owned loaded fragments and
// the retained activation barrier. Guest emptiness is required before stopping
// services and again before stopping the resource slice. Dormancy is observed
// separately after these operations; successful systemctl alone is not proof.
func stopConfigurationRemovalUnitsWith(ctx context.Context, command func(context.Context, string, ...string) *exec.Cmd, plan ConfigurationRemovalPlan, guard, guests func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if command == nil || guard == nil || guests == nil {
		return ErrPlan
	}
	units, err := configurationRemovalUnits(plan)
	if err != nil {
		return err
	}
	phases := [][]string{{}, {}, {}}
	for _, unit := range units {
		phase := 1
		if strings.HasSuffix(unit, ".socket") {
			phase = 0
		} else if unit == "homenode.slice" {
			phase = 2
		}
		phases[phase] = append(phases[phase], unit)
	}
	bounded, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	if err := guard(bounded); err != nil {
		return err
	}
	if err := guests(bounded); err != nil {
		return err
	}
	for index, units := range phases {
		if len(units) == 0 {
			continue
		}
		if err := guard(bounded); err != nil {
			return err
		}
		if index == 2 {
			if err := guests(bounded); err != nil {
				return err
			}
		}
		if err := bounded.Err(); err != nil {
			return err
		}
		step, finish := context.WithTimeout(bounded, 30*time.Second)
		args := append([]string{"--system", "--no-pager", "--no-ask-password", "stop"}, units...)
		cmd := command(step, "/usr/bin/systemctl", args...)
		if cmd == nil {
			finish()
			return ErrPlan
		}
		cmd.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C", "SYSTEMD_COLORS=0", "SYSTEMD_PAGER=cat"}
		cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
		cmd.WaitDelay = time.Second
		err := cmd.Run()
		contextErr := step.Err()
		finish()
		if err != nil || contextErr != nil {
			return errors.Join(ErrConflict, err, contextErr)
		}
		if err := guard(bounded); err != nil {
			return err
		}
	}
	if err := guests(bounded); err != nil {
		return err
	}
	return guard(bounded)
}
