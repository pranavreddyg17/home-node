package backup

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

type privateManagementFixture struct {
	store              *state.Store
	device             string
	captures, confirms int
	failure            error
	substitute         bool
	root               *os.Root
}

func (m *privateManagementFixture) StageManagementSnapshot(ctx context.Context, token, device string, root *os.Root) error {
	m.captures++
	if device != m.device {
		return state.ErrMaintenanceOwner
	}
	m.root = root
	_, err := m.store.MaintenanceRecoverySnapshot(ctx, token, root.Name())
	return err
}
func (m *privateManagementFixture) ConfirmStaging(ctx context.Context, token, device string) error {
	m.confirms++
	if m.substitute {
		if err := m.root.Rename("snapshot.db", "snapshot.original"); err != nil {
			return err
		}
		if err := m.root.WriteFile("snapshot.db", []byte("preserved replacement"), 0600); err != nil {
			return err
		}
	}
	return m.failure
}

func TestPrivateRecoveryStagingConfirmFailureCleansOnlyOwnedSnapshot(t *testing.T) {
	for _, scenario := range []string{"success", "confirmation-failure", "substitution"} {
		t.Run(scenario, func(t *testing.T) {
			store, disks, root, token, policy := recoveryStageFixture(t)
			// A host with no persistent apps still needs its management recovery set.
			if _, err := store.DB.Exec("DELETE FROM apps"); err != nil {
				t.Fatal(err)
			}
			device := state.Random()
			failure := errors.New("confirmation lost")
			management := &privateManagementFixture{store: store, device: device}
			if scenario != "success" {
				management.failure = failure
			}
			management.substitute = scenario == "substitution"
			manifest, err := StagePrivateRecoverySet(context.Background(), management, disks, device, token, disks.token, root, "0.1.0", 1, policy)
			if management.captures != 1 || management.confirms != 1 || disks.calls != 0 {
				t.Fatal("unexpected private staging work", management.captures, management.confirms, disks.calls)
			}
			if scenario == "success" {
				if err != nil || len(manifest.Files) != 1 {
					t.Fatal(manifest, err)
				}
				if err = ValidateRecoverySet(context.Background(), root, manifest, policy); err != nil {
					t.Fatal(err)
				}
			} else {
				if !errors.Is(err, failure) || len(manifest.Files) != 0 {
					t.Fatal("confirmation failure lost", manifest, err)
				}
				data, readErr := root.ReadFile("snapshot.db")
				if scenario == "substitution" {
					if !errors.Is(err, ErrManifest) || readErr != nil || string(data) != "preserved replacement" {
						t.Fatal("substitution not retained/reported", err, readErr)
					}
				} else if !os.IsNotExist(readErr) {
					t.Fatal("owned failed staging retained", readErr)
				}
			}
		})
	}
}
