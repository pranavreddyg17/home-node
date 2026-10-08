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

func applyAccountFixtureAllocator(t *testing.T, engine **Engine, expected GuestUIDAllocationPreview) {
	t.Helper()
	e := *engine
	ctx := context.Background()
	intent, err := e.loadGuestUIDAllocationIntent(ctx, expected.OwnerID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.host.Lstat("etc/.homenode-login-defs.stage"); !os.IsNotExist(err) {
		t.Fatal("native fixture refuses occupied allocator staging", err)
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
			t.Error("allocator fixture cleanup admission", err)
			return
		}
		defer cleanup.Close()
		cleanup.mu.Lock()
		defer cleanup.mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		stage, err := cleanup.loadGuestUIDAllocationStage(ctx, intent)
		if err != nil {
			t.Error("allocator restoration lacks committed staging authority; preserving state", err)
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
			return cleanup.withGuestUIDAllocationIntent(ctx, intent, func(ctx context.Context, checkIntent func() error) error {
				return cleanup.withGuestIdentityRecordGuarded(ctx, "guest-uid-allocation-stage.json", encoded, func(ctx context.Context, checkStage func() error) (result error) {
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
					return withGuestIdentityAllocationLock(ctx, directory, func(ctx context.Context, checkLock func() error) (result error) {
						guard := func(ctx context.Context) error {
							for _, check := range []func() error{checkIntent, checkStage, checkLock} {
								if err := check(); err != nil {
									return err
								}
							}
							if err := runtimeGuard(ctx); err != nil {
								return err
							}
							owner, err := cleanup.inspectGuestIdentityAccountsLocked(ctx)
							if err != nil {
								return err
							}
							opened, statErr := parent.Stat()
							named, nameErr := cleanup.host.Lstat("etc")
							if owner != intent.OwnerID || statErr != nil || nameErr != nil || !os.SameFile(opened, named) {
								return ErrConflict
							}
							return checkLock()
						}
						// Only the two independently journaled inodes are reversible.
						reversed := stage
						reversed.Inode, reversed.SourceInode = stage.SourceInode, stage.Inode
						reversed.Device, reversed.SourceDevice = stage.SourceDevice, stage.Device
						reversed.Bytes = int64(len(intent.Original))
						if err := exchangeGuestUIDAllocationConfiguration(ctx, parent, reversed, []byte(intent.Proposal.Contents), []byte(intent.Original), guard); err != nil {
							return err
						}
						pending, err := directory.OpenFile(".homenode-login-defs.stage", os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
						if err != nil {
							return err
						}
						defer func() { result = errors.Join(result, pending.Close()) }()
						var stat unix.Stat_t
						contents, err := io.ReadAll(io.LimitReader(pending, maxGuestUIDAllocatorConfigurationBytes+1))
						opened, statErr := pending.Stat()
						named, nameErr := directory.Lstat(".homenode-login-defs.stage")
						if err != nil || statErr != nil || nameErr != nil || !os.SameFile(opened, named) || unix.Fstat(int(pending.Fd()), &stat) != nil || uint64(stat.Dev) != stage.Device || stat.Ino != stage.Inode || stat.Mode != unix.S_IFREG|0644 || stat.Uid != 0 || stat.Gid != 0 || stat.Nlink != 1 || string(contents) != intent.Proposal.Contents {
							return ErrConflict
						}
						if err := guard(ctx); err != nil {
							return err
						}
						if err := directory.Remove(".homenode-login-defs.stage"); err != nil {
							return err
						}
						return parent.Sync()
					})
				})
			})
		})
		if err != nil {
			t.Error("native allocator restoration refused; preserving artifacts", err)
			return
		}
		if err := cleanup.journalRoot.Remove("guest-uid-allocation-stage.json"); err != nil {
			t.Error("restored allocator fixture record cleanup", err)
			return
		}
		if err := syncDirectory(cleanup.journalRoot, "."); err != nil {
			t.Error("restored allocator fixture journal durability", err)
		}
	})
	expected.ConfigurationApplied = true
	for i := 0; i < 2; i++ {
		actual, err := e.ApplyGuestUIDAllocationConfiguration(ctx)
		if err != nil || actual != expected {
			t.Fatal("native allocator application or retry", err, actual)
		}
	}
	current, err := os.ReadFile("/etc/login.defs")
	retained, retainedErr := os.ReadFile("/etc/.homenode-login-defs.stage")
	if err != nil || retainedErr != nil || string(current) != intent.Proposal.Contents || string(retained) != intent.Original {
		t.Fatal("native allocator exchange did not retain approved bytes", err, retainedErr)
	}
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	*engine = nil
	output, err := accountCommand(ctx, "/usr/bin/homenode", "guest-allocation-apply")
	if err != nil {
		t.Fatal("packaged allocator application retry", err)
	}
	var actual GuestUIDAllocationPreview
	if err := json.Unmarshal(output, &actual); err != nil || actual != expected {
		t.Fatal("packaged allocator application status", err)
	}
	*engine, err = Open("/", "/var/lib/homenode-install")
	if err != nil {
		t.Fatal(err)
	}
	after, err := (*engine).journalRoot.Lstat("recovery-blocked")
	if err != nil || !os.SameFile(marker, after) {
		t.Fatal("native allocator application released or replaced activation marker", err)
	}
}
