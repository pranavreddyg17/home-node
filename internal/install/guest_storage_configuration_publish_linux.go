//go:build linux

package install

import (
	"context"
	"encoding/json"
	"os"
	"reflect"

	"golang.org/x/sys/unix"
)

// Caller holds e.mu and retains qualified directory, installation/storage,
// account and dormant-runtime authority through guard. This private operation
// publishes both configurations and their destination install journal; it does
// not release the activation barrier or activate a production installer command.
func (e *Engine) publishGuestStorageConfigurationLocked(ctx context.Context, directory *os.File, plan GuestStorageProvisioningPlan, guard func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if directory == nil || guard == nil {
		return ErrPlan
	}
	if err := guard(ctx); err != nil {
		return err
	}
	if err := e.checkGuestStorageConfigurationDirectory(ctx, directory); err != nil {
		return err
	}
	current, err := e.load()
	if err != nil {
		return err
	}
	return e.withGuestStorageConfigurationIntent(ctx, current, plan, func(intent guestStorageConfigurationIntent, checkIntent func() error) error {
		stage, err := e.loadGuestStorageConfigurationStage(ctx, current, plan)
		if err != nil {
			return err
		}
		encoded, err := json.Marshal(stage)
		if err != nil {
			return err
		}
		return e.withGuestIdentityRecordGuarded(ctx, "guest-storage-configuration-stage.json", encoded, func(ctx context.Context, checkStage func() error) error {
			check := func(ctx context.Context) error {
				if err := e.checkGuestStorageConfigurationDirectory(ctx, directory); err != nil {
					return err
				}
				if err := guard(ctx); err != nil {
					return err
				}
				if err := checkIntent(); err != nil {
					return err
				}
				if err := checkStage(); err != nil {
					return err
				}
				journal, err := e.load()
				if err != nil {
					return err
				}
				if !reflect.DeepEqual(journal, intent.Original) && !reflect.DeepEqual(journal, intent.Desired) {
					return ErrConflict
				}
				if err := e.admitGuestStorageConfigurationInstallation(ctx, journal, intent, stage, directory); err != nil {
					return err
				}
				if err := guard(ctx); err != nil {
					return err
				}
				return e.checkGuestStorageConfigurationDirectory(ctx, directory)
			}
			completed := func() error {
				if err := check(ctx); err != nil {
					return err
				}
				originals := [][]byte{intent.SourcePolicy, intent.SourceEnvironment}
				desired := [][]byte{intent.Policy, intent.Environment}
				for i, receipt := range stage.Files {
					if err := reconcileGuestStorageConfigurationFile(ctx, directory, receipt, originals[i], desired[i], check, false); err != nil {
						return err
					}
				}
				names := []string{"runtime-policy.json", "services.env"}
				for i, name := range names {
					receipt := stage.Files[i]
					var named unix.Stat_t
					mode := uint32(0644)
					if i == 0 {
						mode = 0600
					}
					if unix.Fstatat(int(directory.Fd()), name, &named, unix.AT_SYMLINK_NOFOLLOW) != nil || uint64(named.Dev) != receipt.Device || named.Ino != receipt.Inode || named.Mode != unix.S_IFREG|mode || named.Uid != 0 || named.Gid != 0 || named.Nlink != 1 || named.Size != receipt.Bytes {
						return ErrConflict
					}
				}
				return check(ctx)
			}
			// A committed destination journal admits only completed host publication.
			// Do not repair a reverted namespace using stale transition authority.
			if reflect.DeepEqual(current, intent.Desired) {
				return completed()
			}
			if err := exchangeGuestStorageConfigurationBatch(ctx, directory, stage, intent, check); err != nil {
				return err
			}
			if err := completed(); err != nil {
				return err
			}
			observed, err := e.load()
			if err != nil {
				return err
			}
			if !reflect.DeepEqual(observed, intent.Desired) {
				if err := e.save(intent.Desired); err != nil {
					return err
				}
			}
			if err := completed(); err != nil {
				return err
			}
			observed, err = e.load()
			if err != nil {
				return err
			}
			if !reflect.DeepEqual(observed, intent.Desired) {
				return ErrConflict
			}
			return check(ctx)
		})
	})
}
