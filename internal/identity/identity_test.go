package identity

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func testService(t *testing.T) *Service {
	t.Helper()
	store, err := state.Open(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	s, err := New(store, "http://localhost:8787")
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func testDevice(t *testing.T, s *Service, caps []string) (Session, string) {
	t.Helper()
	id, token := state.Random(), state.Random()
	data, _ := json.Marshal(caps)
	err := s.Store.Transaction(context.Background(), func(tx *sql.Tx) error {
		if _, err := tx.Exec("INSERT INTO devices(id,name,capabilities,created_at) VALUES(?,'test',?,?)", id, string(data), time.Now().Unix()); err != nil {
			return err
		}
		return insertSession(tx, token, id, 1)
	})
	if err != nil {
		t.Fatal(err)
	}
	session, err := s.Authenticate(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	return session, token
}

func TestSetupCodeSingleUseAndRotation(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	old, err := s.CreateSetupCode(ctx)
	if err != nil {
		t.Fatal(err)
	}
	code, err := s.CreateSetupCode(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.BeginRegistration(ctx, old, "laptop"); !errors.Is(err, ErrDenied) {
		t.Fatalf("old code accepted: %v", err)
	}
	_, challenge, err := s.BeginRegistration(ctx, code, "laptop")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.BeginRegistration(ctx, code, "laptop"); !errors.Is(err, ErrDenied) {
		t.Fatalf("replay accepted: %v", err)
	}
	if _, _, _, err = s.takeChallenge(ctx, challenge); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = s.takeChallenge(ctx, challenge); !errors.Is(err, ErrDenied) {
		t.Fatalf("challenge replay accepted: %v", err)
	}
}
func TestExpiredEnrollment(t *testing.T) {
	s := testService(t)
	code, err := s.CreateSetupCode(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Store.DB.Exec("UPDATE invitations SET expires_at=0"); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.BeginRegistration(context.Background(), code, "laptop"); !errors.Is(err, ErrDenied) {
		t.Fatalf("expired code accepted: %v", err)
	}
}
func TestSessionRevocationAndRecoveryEpoch(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	admin, _ := testDevice(t, s, AllCapabilities)
	device, token := testDevice(t, s, []string{"files"})
	if err := s.Revoke(ctx, admin, device.Device.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(ctx, token); !errors.Is(err, ErrDenied) {
		t.Fatalf("revoked token accepted: %v", err)
	}
	_, another := testDevice(t, s, []string{"ai"})
	if _, err := s.Store.DB.Exec("UPDATE identity SET epoch=epoch+1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(ctx, another); !errors.Is(err, ErrDenied) {
		t.Fatalf("old epoch accepted: %v", err)
	}
}
func TestLastAdminAndCapabilityBoundary(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	admin, _ := testDevice(t, s, AllCapabilities)
	limited, _ := testDevice(t, s, []string{"files"})
	if err := s.Revoke(ctx, admin, admin.Device.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("last admin removed: %v", err)
	}
	if _, err := s.Pair(ctx, limited, "phone", AllCapabilities); !errors.Is(err, ErrDenied) {
		t.Fatalf("non-admin widened access: %v", err)
	}
	admin.VerifiedAt = time.Now().Add(-10 * time.Minute).Unix()
	if _, err := s.Pair(ctx, admin, "phone", AllCapabilities); !errors.Is(err, ErrDenied) {
		t.Fatalf("stale verification accepted: %v", err)
	}
}
func TestRevocationCancelsOutstandingPairing(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	admin, _ := testDevice(t, s, AllCapabilities)
	other, _ := testDevice(t, s, AllCapabilities)
	if _, err := s.Store.DB.Exec("UPDATE identity SET claimed=1"); err != nil {
		t.Fatal(err)
	}
	code, err := s.Pair(ctx, admin, "phone", []string{"files"})
	if err != nil {
		t.Fatal(err)
	}
	_, challenge, err := s.BeginRegistration(ctx, code, "phone")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Pair(ctx, admin, "other", []string{"files"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Revoke(ctx, other, admin.Device.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = s.takeChallenge(ctx, challenge); !errors.Is(err, ErrDenied) {
		t.Fatalf("pending ceremony survived issuer revocation: %v", err)
	}
	if _, _, err = s.BeginRegistration(ctx, second, "phone"); !errors.Is(err, ErrDenied) {
		t.Fatalf("invitation survived issuer revocation: %v", err)
	}
}
func TestIdleAndAbsoluteExpiry(t *testing.T) {
	for _, column := range []string{"last_seen", "expires_at"} {
		t.Run(column, func(t *testing.T) {
			s := testService(t)
			_, token := testDevice(t, s, AllCapabilities)
			if _, err := s.Store.DB.Exec("UPDATE sessions SET " + column + "=0"); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Authenticate(context.Background(), token); !errors.Is(err, ErrDenied) {
				t.Fatalf("expired session accepted: %v", err)
			}
		})
	}
}
func TestRecoveryCodeIsOneUse(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	code := state.Random()
	if _, err := s.Store.DB.Exec("UPDATE identity SET claimed=1; INSERT INTO recovery_codes VALUES(?)", state.Hash(code)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Recovery(ctx, code); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Recovery(ctx, code); !errors.Is(err, ErrDenied) {
		t.Fatalf("recovery replay accepted: %v", err)
	}
}
