package runtimeclient

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/backup"
	"github.com/pranavreddyg17/home-node/internal/state"
)

type snapshotAuthorityFixture struct {
	request backup.SnapshotPageRequest
	err     error
	calls   int
	check   func()
}

func (a *snapshotAuthorityFixture) VerifySnapshotPage(_ context.Context, request backup.SnapshotPageRequest) error {
	a.calls++
	if request != a.request {
		return errors.New("foreign selector request")
	}
	if a.check != nil {
		a.check()
	}
	return a.err
}

type snapshotReaderFixture struct {
	cursor string
	page   backup.SnapshotPage
	err    error
	calls  int
}

func (r *snapshotReaderFixture) SnapshotPage(_ context.Context, cursor string) (backup.SnapshotPage, error) {
	r.calls++
	if cursor != r.cursor {
		return backup.SnapshotPage{}, errors.New("foreign cursor")
	}
	return r.page, r.err
}

func TestSnapshotWorkerAuthorizesBeforeCredentialRead(t *testing.T) {
	credential, err := os.CreateTemp(t.TempDir(), "credential")
	if err != nil {
		t.Fatal(err)
	}
	request := backup.SnapshotPageRequest{Version: 4, Kind: "snapshot-page", RequestID: state.Random(), DeviceID: state.Random()}
	denied := errors.New("owner session withdrawn")
	authority := &snapshotAuthorityFixture{request: request, err: denied}
	config := BackupWorkerConfig{RepositoryTarget: backup.Target{MountPath: "/mnt/homenode-backup", UUID: "fixture-drive", RepositoryID: state.Hash("repository")}}
	page, err := RunCredentialedSnapshotPage(context.Background(), request, config, credential, authority)
	if !errors.Is(err, denied) || authority.calls != 1 || page.Snapshots != nil || page.Next != "" {
		t.Fatal("authorization did not precede credential qualification", page, err, authority.calls)
	}
	if _, err := credential.Stat(); err == nil {
		t.Fatal("worker retained owned credential")
	}
}

func TestSnapshotWorkerRechecksAuthorityBeforeReturningCandidates(t *testing.T) {
	request := backup.SnapshotPageRequest{Version: 4, Kind: "snapshot-page", RequestID: state.Random(), DeviceID: state.Random(), Cursor: state.Hash("cursor")}
	for _, scenario := range []string{"valid", "withdrawn", "cancelled", "repository-error"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			authority := &snapshotAuthorityFixture{request: request}
			reader := &snapshotReaderFixture{cursor: request.Cursor, page: backup.SnapshotPage{Snapshots: []backup.SnapshotReference{}, Next: ""}}
			if scenario == "withdrawn" {
				authority.err = errors.New("revoked")
			}
			if scenario == "cancelled" {
				authority.check = cancel
			}
			if scenario == "repository-error" {
				reader.err = errors.New("repository failed")
			}
			page, err := authorizedSnapshotPage(ctx, request, authority, reader)
			if reader.calls != 1 {
				t.Fatal("listing not called exactly once")
			}
			if scenario == "valid" {
				if err != nil || page.Snapshots == nil || authority.calls != 1 {
					t.Fatal(page, err, authority.calls)
				}
			} else if err == nil || page.Snapshots != nil || page.Next != "" {
				t.Fatal("unqualified page disclosed", page, err)
			}
			if scenario == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatal("cancellation lost", err)
			}
			if scenario == "repository-error" && authority.calls != 0 {
				t.Fatal("failed listing reached authority recheck")
			}
		})
	}
}
