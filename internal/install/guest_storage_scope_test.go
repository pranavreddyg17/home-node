package install

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

func TestRootGuestStorageScopeRetainsIntentDuringRuntimeObservation(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root-only owned fixture")
	}
	c, _, _, now := configurationFixture(t)
	c.Maintenance = &MaintenanceAccount{UID: 803, GID: 803}
	c.BackupRepositoryID, c.BackupRelease, c.BackupDriveUUID = strings.Repeat("a", 64), "0.1.0", "abcd-1234"
	preview, err := ConfigurationPlan(c, now)
	if err != nil {
		t.Fatal(err)
	}
	host, journal := roots(t)
	e := openEngine(t, host, journal)
	defer e.Close()
	ctx := context.Background()
	if err := e.Apply(ctx, preview.Plan); err != nil {
		t.Fatal(err)
	}
	if err := e.blockRecoveryActivation(ctx); err != nil {
		t.Fatal(err)
	}
	identity, err := guestUIDProvisioningPlan(ctx, strings.Repeat("a", 32), supervisor.GuestUIDPool{First: 200000, Last: 200002}, []uint32{1001, 1002})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := guestStorageProvisioningPlan(ctx, identity, 993, []int{1001, 1002, 1003})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.commitGuestStorageIntent(ctx, plan); err != nil {
		t.Fatal(err)
	}
	called := false
	if err := e.withGuestStorageExclusionLocked(ctx, func(context.Context) error { return nil }, e.observeRecoveryDestinationVacancy, func(ctx context.Context, p GuestStorageProvisioningPlan, guard func(context.Context) error) error {
		called = true
		return guard(ctx)
	}); err != nil || !called {
		t.Fatal("retained storage scope", called, err)
	}
	data, err := e.journalRoot.ReadFile("guest-storage-intent.json")
	if err != nil {
		t.Fatal(err)
	}
	observations := 0
	called = false
	err = e.withGuestStorageExclusionLocked(ctx, func(context.Context) error {
		observations++
		if observations == 1 {
			if err := e.journalRoot.Rename("guest-storage-intent.json", "guest-storage-intent.old"); err != nil {
				return err
			}
			return e.journalRoot.WriteFile("guest-storage-intent.json", data, 0600)
		}
		return nil
	}, e.observeRecoveryDestinationVacancy, func(context.Context, GuestStorageProvisioningPlan, func(context.Context) error) error {
		called = true
		return nil
	})
	if !errors.Is(err, ErrConflict) || called {
		t.Fatal("runtime observation replaced intent before consumer", called, err)
	}
	if err := e.requireRecoveryActivationBlock(ctx); err != nil {
		t.Fatal("refusal removed activation block", err)
	}
}
