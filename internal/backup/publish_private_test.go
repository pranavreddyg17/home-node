package backup

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

type publicationManagementFixture struct {
	confirms int
	failure  error
}

func (m *publicationManagementFixture) BeginPublishing(context.Context, string, string) error {
	return nil
}
func (m *publicationManagementFixture) ConfirmPublishing(context.Context, string, string) error {
	m.confirms++
	if m.confirms == 2 {
		return m.failure
	}
	return nil
}

type publicationRepositoryFixture struct{ calls int }

func (r *publicationRepositoryFixture) Snapshot(context.Context, *os.File, Manifest, RestorePolicy) (string, error) {
	r.calls++
	return state.Hash("private published snapshot"), nil
}

func TestPrivatePublicationPreservesSnapshotAfterConfirmationFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		store, disks, root, token, policy := recoveryStageFixture(t)
		manifest, err := StageRecoverySet(context.Background(), store, disks, token, disks.token, root, "0.1.0", 1, policy)
		if err != nil {
			t.Fatal(err)
		}
		failure := errors.New("publishing ownership confirmation failed")
		management := &publicationManagementFixture{}
		if fail {
			management.failure = failure
		}
		repository := &publicationRepositoryFixture{}
		snapshot, err := PublishPrivateRecoverySet(context.Background(), management, repository, state.Random(), token, root, manifest, policy)
		if snapshot != state.Hash("private published snapshot") || repository.calls != 1 || management.confirms != 2 {
			t.Fatal("publication outcome lost", snapshot, err)
		}
		if fail && !errors.Is(err, failure) || !fail && err != nil {
			t.Fatal("confirmation outcome lost", err)
		}
	}
}
