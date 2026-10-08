//go:build linux

package install

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func applyAccountFixtureIdentity(t *testing.T, engine **Engine, expected GuestIdentityConfigurationPreview) {
	t.Helper()
	e := *engine
	ctx := context.Background()
	intent, err := e.loadGuestIdentityNameServiceIntent(ctx, expected.OwnerID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.host.Lstat("etc/.homenode-nsswitch.stage"); !os.IsNotExist(err) {
		t.Fatal("native fixture refuses occupied NSS staging", err)
	}
	marker, err := e.journalRoot.Lstat("recovery-blocked")
	if err != nil {
		t.Fatal(err)
	}
	// Registered after configuration cleanup, so restoration runs before units
	// and the activation marker are removed. Failures preserve both artifacts.
	t.Cleanup(func() {
		cleanup, err := Open("/", "/var/lib/homenode-install")
		if err != nil {
			t.Error("NSS fixture cleanup admission", err)
			return
		}
		defer cleanup.Close()
		cleanup.mu.Lock()
		defer cleanup.mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		stage, err := cleanup.loadGuestIdentityNameServiceStage(ctx, intent)
		if err != nil {
			t.Error("NSS restoration lacks committed staging authority; preserving state", err)
			return
		}
		encoded, err := json.Marshal(stage)
		if err != nil {
			t.Error(err)
			return
		}
		observe := func(ctx context.Context) error {
			if err := ObserveRecoveryActivationConditions(ctx); err != nil {
				return err
			}
			if err := ObserveRecoveryServicesDormant(ctx); err != nil {
				return err
			}
			return ObserveRecoveryGuestsEmpty(ctx)
		}
		err = cleanup.withRecoveryExclusionGuardedLocked(ctx, observe, cleanup.observeRecoveryDestinationVacancy, func(ctx context.Context, runtimeGuard func(context.Context) error) error {
			return cleanup.withGuestIdentityNameServiceIntentGuarded(ctx, intent, func(ctx context.Context, checkIntent func() error) error {
				return cleanup.withGuestIdentityRecordGuarded(ctx, "guest-identity-nss-stage.json", encoded, func(ctx context.Context, checkStage func() error) (result error) {
					directory, err := cleanup.host.OpenRoot("etc")
					if err != nil {
						return err
					}
					defer func() { result = errors.Join(result, directory.Close()) }()
					parent, err := directory.Open(".")
					if err != nil {
						return err
					}
					defer func() { result = errors.Join(result, parent.Close()) }()
					return withGuestIdentityAllocationLock(ctx, directory, func(ctx context.Context, checkLock func() error) error {
						guard := func(ctx context.Context) error {
							for _, check := range []func() error{checkIntent, checkStage, checkLock} {
								if err := check(); err != nil {
									return err
								}
							}
							return runtimeGuard(ctx)
						}
						// Only the two independently journaled inodes are reversible.
						reversed := stage
						reversed.Inode, reversed.SourceInode = stage.SourceInode, stage.Inode
						reversed.Device, reversed.SourceDevice = stage.SourceDevice, stage.Device
						reversed.Bytes = int64(len(intent.Original))
						if err := exchangeGuestIdentityConfiguration(ctx, parent, reversed, []byte(intent.Proposal.Contents), []byte(intent.Original), guard); err != nil {
							return err
						}
						pending, err := directory.OpenFile(".homenode-nsswitch.stage", os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
						if err != nil {
							return err
						}
						defer pending.Close()
						var stat unix.Stat_t
						contents, err := io.ReadAll(io.LimitReader(pending, 8193))
						opened, statErr := pending.Stat()
						named, nameErr := directory.Lstat(".homenode-nsswitch.stage")
						if err != nil || statErr != nil || nameErr != nil || !os.SameFile(opened, named) || unix.Fstat(int(pending.Fd()), &stat) != nil || uint64(stat.Dev) != stage.Device || stat.Ino != stage.Inode || stat.Mode != unix.S_IFREG|0644 || stat.Uid != 0 || stat.Gid != 0 || stat.Nlink != 1 || string(contents) != intent.Proposal.Contents {
							return ErrConflict
						}
						if err := guard(ctx); err != nil {
							return err
						}
						if err := directory.Remove(".homenode-nsswitch.stage"); err != nil {
							return err
						}
						return parent.Sync()
					})
				})
			})
		})
		if err != nil {
			t.Error("native NSS restoration refused; preserving artifacts", err)
			return
		}
		if err := cleanup.journalRoot.Remove("guest-identity-nss-stage.json"); err != nil {
			t.Error("restored NSS fixture record cleanup", err)
			return
		}
		if err := syncDirectory(cleanup.journalRoot, "."); err != nil {
			t.Error("restored NSS fixture journal durability", err)
		}
	})
	expected.ConfigurationApplied = true
	for i := 0; i < 2; i++ {
		actual, err := e.ApplyGuestIdentityConfiguration(ctx)
		if err != nil || actual != expected {
			t.Fatal("native identity application or retry", err, actual)
		}
	}
	current, err := os.ReadFile("/etc/nsswitch.conf")
	retained, retainedErr := os.ReadFile("/etc/.homenode-nsswitch.stage")
	if err != nil || retainedErr != nil || string(current) != intent.Proposal.Contents || string(retained) != intent.Original {
		t.Fatal("native identity exchange did not retain approved bytes", err, retainedErr)
	}
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	*engine = nil
	output, err := accountCommand(ctx, "/usr/bin/homenode", "guest-identity-apply")
	if err != nil {
		t.Fatal("packaged identity application retry", err)
	}
	var actual GuestIdentityConfigurationPreview
	if err := json.Unmarshal(output, &actual); err != nil || actual != expected {
		t.Fatal("packaged identity application status", err)
	}
	*engine, err = Open("/", "/var/lib/homenode-install")
	if err != nil {
		t.Fatal(err)
	}
	after, err := (*engine).journalRoot.Lstat("recovery-blocked")
	if err != nil || !os.SameFile(marker, after) {
		t.Fatal("native identity application released or replaced activation marker", err)
	}
}
