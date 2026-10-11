//go:build linux

package install

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"time"
)

// RetireConfigurationUnits reloads the manager after completed file removal and
// proves owned units absent. It retains data, identities, package and activation
// exclusion. This is not a complete uninstall or activation release.
func (e *Engine) RetireConfigurationUnits(ctx context.Context) error {
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
	if !intent.Ready {
		return ErrConflict
	}
	retired := false
	observe := func(ctx context.Context) error {
		if retired {
			if err := e.observeConfigurationRemovalRetiredWith(ctx, exec.CommandContext); err != nil {
				return err
			}
		} else {
			if err := e.observeConfigurationRemovalManagerWith(ctx, exec.CommandContext); err != nil {
				return err
			}
		}
		return observeConfigurationRemovalGuestsEmpty(ctx)
	}
	return e.withConfigurationRemovalExclusionLocked(ctx, observe, func(ctx context.Context, guard func(context.Context) error) error {
		return reloadConfigurationRemovalManagerWith(ctx, exec.CommandContext, guard, func(ctx context.Context) error { retired = true; return guard(ctx) })
	})
}

func reloadConfigurationRemovalManagerWith(ctx context.Context, command func(context.Context, string, ...string) *exec.Cmd, before, after func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if command == nil || before == nil || after == nil {
		return ErrPlan
	}
	if err := before(ctx); err != nil {
		return err
	}
	bounded, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := bounded.Err(); err != nil {
		return err
	}
	cmd := command(bounded, "/usr/bin/systemctl", "--system", "--no-pager", "--no-ask-password", "daemon-reload")
	if cmd == nil {
		return ErrPlan
	}
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C", "SYSTEMD_COLORS=0", "SYSTEMD_PAGER=cat"}
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	cmd.WaitDelay = time.Second
	if err := cmd.Run(); err != nil {
		return errors.Join(ErrConflict, err, bounded.Err())
	}
	if err := bounded.Err(); err != nil {
		return err
	}
	return after(ctx)
}
