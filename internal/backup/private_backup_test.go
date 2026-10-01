package backup

import (
	"context"
	"errors"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

type privateBackupFixture struct {
	*privateManagementFixture
	publication publicationManagementFixture
}

func (m *privateBackupFixture) BeginPublishing(ctx context.Context, token, device string) error {
	return m.publication.BeginPublishing(ctx, token, device)
}
func (m *privateBackupFixture) ConfirmPublishing(ctx context.Context, token, device string) error {
	return m.publication.ConfirmPublishing(ctx, token, device)
}
func (m *privateBackupFixture) ClaimPublication(ctx context.Context, token, device string) error {
	return m.publication.ClaimPublication(ctx, token, device)
}
func (m *privateBackupFixture) RecordPublication(ctx context.Context, token, device, snapshot string) error {
	return m.publication.RecordPublication(ctx, token, device, snapshot)
}

func TestPrivateBackupCombinesStagingWithClaimedPublication(t *testing.T) {
	for _, scenario := range []string{"success", "staging-failure", "claim-failure", "acknowledgement-failure"} {
		t.Run(scenario, func(t *testing.T) {
			store, disks, staging, token, policy := recoveryStageFixture(t)
			if _, err := store.DB.Exec("DELETE FROM apps"); err != nil {
				t.Fatal(err)
			}
			device := state.Random()
			failure := errors.New("fixture failure")
			management := &privateBackupFixture{privateManagementFixture: &privateManagementFixture{store: store, device: device}}
			switch scenario {
			case "staging-failure":
				management.privateManagementFixture.failure = failure
			case "claim-failure":
				management.publication.claimFailure = failure
			case "acknowledgement-failure":
				management.publication.recordFailure = failure
			}
			repository := &publicationRepositoryFixture{}
			snapshot, err := RunPrivateBackup(context.Background(), management, disks, repository, device, token, disks.token, staging, "0.1.0", 1, policy)
			if scenario == "success" {
				if err != nil || snapshot != state.Hash("private published snapshot") {
					t.Fatal(snapshot, err)
				}
			} else if !errors.Is(err, failure) {
				t.Fatal("failure lost", err)
			}
			if scenario == "staging-failure" || scenario == "claim-failure" {
				if repository.calls != 0 || snapshot != "" || management.publication.records != 0 {
					t.Fatal("failed admission published")
				}
			} else if repository.calls != 1 || snapshot == "" || management.publication.records != 1 {
				t.Fatal("repository outcome lost")
			}
			if err = store.Transaction(context.Background(), state.RequireAdmission); !errors.Is(err, state.ErrMaintenance) {
				t.Fatal("worker released coordinator barrier", err)
			}
		})
	}
}
