package control

import (
	"context"
	"errors"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/identity"
	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestPendingSnapshotRequestBindsExactLiveSession(t *testing.T) {
	for _, scenario := range []string{"valid", "cursor", "device", "request", "kind", "release", "cancel", "logout", "revoked"} {
		t.Run(scenario, func(t *testing.T) {
			s := testServer(t)
			token := seedBackupSession(t, s)
			if _, err := s.Store.DB.Exec("UPDATE identity SET claimed=1"); err != nil {
				t.Fatal(err)
			}
			actor, err := s.Identity.Authenticate(context.Background(), token)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			request, release, err := s.beginSnapshotRequest(ctx, actor, "")
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			if err := s.verifySnapshotRequest(ctx, request); err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "cursor":
				request.Cursor = state.Hash("foreign cursor")
			case "device":
				request.DeviceID = state.Random()
			case "request":
				request.RequestID = state.Random()
			case "kind":
				request.Kind = "cleanup"
			case "release":
				release()
			case "cancel":
				cancel()
			case "logout":
				err = s.Identity.Logout(context.Background(), actor)
			case "revoked":
				_, err = s.Store.DB.Exec("UPDATE devices SET revoked_at=1 WHERE id=?", actor.Device.ID)
			}
			if err != nil {
				t.Fatal(err)
			}
			err = s.verifySnapshotRequest(context.Background(), request)
			if scenario == "valid" {
				if err != nil {
					t.Fatal(err)
				}
			} else if !errors.Is(err, identity.ErrDenied) {
				t.Fatal("unbound request admitted", err)
			}
		})
	}
}

func TestPendingSnapshotRequestsBoundConcurrencyAndWithdraw(t *testing.T) {
	s := testServer(t)
	token := seedBackupSession(t, s)
	if _, err := s.Store.DB.Exec("UPDATE identity SET claimed=1"); err != nil {
		t.Fatal(err)
	}
	actor, err := s.Identity.Authenticate(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	releases := []func(){}
	for i := 0; i < 4; i++ {
		_, release, err := s.beginSnapshotRequest(context.Background(), actor, "")
		if err != nil {
			t.Fatal(err)
		}
		releases = append(releases, release)
		defer release()
	}
	if _, release, err := s.beginSnapshotRequest(context.Background(), actor, ""); !errors.Is(err, identity.ErrDenied) || release != nil {
		t.Fatal("pending request limit bypassed", err)
	}
	releases[0]()
	request, release, err := s.beginSnapshotRequest(context.Background(), actor, state.Hash("cursor"))
	if err != nil {
		t.Fatal("released slot not reusable", err)
	}
	defer release()
	if err := s.verifySnapshotRequest(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	actor.Device.Capabilities[0] = "files"
	if err := s.verifySnapshotRequest(context.Background(), request); err != nil {
		t.Fatal("caller mutated retained capabilities", err)
	}
}
