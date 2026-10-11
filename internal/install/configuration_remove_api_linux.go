//go:build linux

package install

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// RemoveConfiguration applies committed removal intent to an owned installed
// host. It retains the activation block, installer journals, service identities,
// all directories, application data and backups. Package removal, manager reload
// and identity retirement are separate uninstall phases.
func (e *Engine) RemoveConfiguration(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if os.Geteuid() != 0 || e.host.Name() != "/" {
		return ErrAccounts
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	if !e.mu.TryLock() {
		return ErrConflict
	}
	defer e.mu.Unlock()
	installed, err := e.load()
	if err != nil {
		return err
	}
	intent, err := e.loadConfigurationRemovalIntent(installed)
	if err != nil {
		return err
	}
	stopped := false
	observe := func(ctx context.Context) error {
		if stopped {
			if err := e.observeConfigurationRemovalManagerWith(ctx, exec.CommandContext); err != nil {
				return err
			}
		} else {
			if err := e.observeConfigurationRemovalLoadedWith(ctx, exec.CommandContext); err != nil {
				return err
			}
		}
		return observeConfigurationRemovalGuestsEmpty(ctx)
	}
	return e.withConfigurationRemovalExclusionLocked(ctx, observe, func(ctx context.Context, guard func(context.Context) error) error {
		// A retry does not issue stop for units already durably absent. The full
		// observer still verifies those units' dormant/not-found manager state.
		stopPlan := intent.Plan
		stopPlan.Files = []ConfigurationRemovalFile{}
		for _, file := range intent.Plan.Files {
			_, err := e.host.Lstat(file.Path)
			if err == nil {
				stopPlan.Files = append(stopPlan.Files, file)
			} else if !os.IsNotExist(err) {
				return err
			}
		}
		if err := stopConfigurationRemovalUnitsWith(ctx, exec.CommandContext, stopPlan, guard, observeConfigurationRemovalGuestsEmpty); err != nil {
			return fmt.Errorf("configuration removal shutdown: %w", err)
		}
		stopped = true
		if err := guard(ctx); err != nil {
			return err
		}
		return e.removeConfigurationLocked(ctx, guard)
	})
}
