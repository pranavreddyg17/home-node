//go:build linux

package install

import (
	"bytes"
	"context"
	"golang.org/x/sys/unix"
	"os"
	"reflect"
)

// Read-only admission for the recovery exclusion's installation observer.
// Caller retains the independently selected plan and both immutable records.
// Only the recorded configuration transition is exempt from ordinary matches.
func (e *Engine) admitGuestStorageConfigurationInstallation(ctx context.Context, current journal, intent guestStorageConfigurationIntent, stage guestStorageConfigurationStage, directory *os.File) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if intent.Version != 1 || directory == nil {
		return ErrPlan
	}
	desired, policy, environment, err := planGuestStorageConfiguration(ctx, intent.Original, intent.SourcePolicy, intent.SourceEnvironment, intent.Plan)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(desired, intent.Desired) || !bytes.Equal(policy, intent.Policy) || !bytes.Equal(environment, intent.Environment) || (!reflect.DeepEqual(current, intent.Original) && !reflect.DeepEqual(current, intent.Desired)) {
		return ErrConflict
	}
	guard := func(ctx context.Context) error { return e.checkGuestStorageConfigurationDirectory(ctx, directory) }
	if err := reconcileGuestStorageConfigurationBatch(ctx, directory, stage, intent, guard, false); err != nil {
		return err
	}
	if reflect.DeepEqual(current, intent.Desired) {
		for i, name := range []string{"runtime-policy.json", "services.env"} {
			var named unix.Stat_t
			receipt := stage.Files[i]
			if unix.Fstatat(int(directory.Fd()), name, &named, unix.AT_SYMLINK_NOFOLLOW) != nil || uint64(named.Dev) != receipt.Device || named.Ino != receipt.Inode {
				return ErrConflict
			}
		}
	}
	for _, item := range current.Items {
		if item.Path == "etc/homenode/runtime-policy.json" || item.Path == "etc/homenode/services.env" {
			continue
		}
		if err := e.matches(item); err != nil {
			return ErrConflict
		}
	}
	observed, err := e.load()
	if err != nil || !reflect.DeepEqual(observed, current) {
		return ErrConflict
	}
	return guard(ctx)
}
