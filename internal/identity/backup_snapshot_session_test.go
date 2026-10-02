package identity

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSnapshotSessionRechecksCurrentOwnerAuthority(t *testing.T) {
	for _, scenario := range []string{"valid", "logout", "revoked", "epoch", "capabilities", "verification", "expired", "idle", "unclaimed", "stale", "forged"} {
		t.Run(scenario, func(t *testing.T) {
			s := testService(t)
			actor, _ := testDevice(t, s, AllCapabilities)
			if _, err := s.Store.DB.Exec("UPDATE identity SET claimed=1"); err != nil {
				t.Fatal(err)
			}
			var err error
			switch scenario {
			case "logout":
				err = s.Logout(context.Background(), actor)
			case "revoked":
				_, err = s.Store.DB.Exec("UPDATE devices SET revoked_at=1 WHERE id=?", actor.Device.ID)
			case "epoch":
				_, err = s.Store.DB.Exec("UPDATE identity SET epoch=epoch+1")
			case "capabilities":
				_, err = s.Store.DB.Exec(`UPDATE devices SET capabilities='["files"]' WHERE id=?`, actor.Device.ID)
			case "verification":
				_, err = s.Store.DB.Exec("UPDATE sessions SET verified_at=verified_at+1 WHERE token_hash=?", actor.TokenHash)
			case "expired":
				_, err = s.Store.DB.Exec("UPDATE sessions SET expires_at=0 WHERE token_hash=?", actor.TokenHash)
			case "idle":
				_, err = s.Store.DB.Exec("UPDATE sessions SET last_seen=0 WHERE token_hash=?", actor.TokenHash)
			case "unclaimed":
				_, err = s.Store.DB.Exec("UPDATE identity SET claimed=0")
			case "stale":
				actor.VerifiedAt = time.Now().Add(-6 * time.Minute).Unix()
			case "forged":
				actor.TokenHash = "foreign"
			}
			if err != nil {
				t.Fatal(err)
			}
			err = s.VerifyBackupSnapshotSession(context.Background(), actor)
			if scenario == "valid" {
				if err != nil {
					t.Fatal(err)
				}
			} else if !errors.Is(err, ErrDenied) {
				t.Fatal("changed authority accepted", err)
			}
		})
	}
}
