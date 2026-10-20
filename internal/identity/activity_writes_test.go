package identity

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRepeatedAuthenticationAvoidsIdenticalActivityWritesWithoutCachingAuthority(t *testing.T) {
	s := testService(t)
	actor, token := testDevice(t, s, []string{"files"})
	ctx := context.Background()
	if _, err := s.Store.DB.Exec("UPDATE sessions SET last_seen=? WHERE token_hash=?", time.Now().Unix()-10, actor.TokenHash); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		"CREATE TABLE activity_write_audit(seen INTEGER NOT NULL)",
		"CREATE TRIGGER audit_activity_updates AFTER UPDATE OF last_seen ON sessions BEGIN INSERT INTO activity_write_audit VALUES(NEW.last_seen); END",
	} {
		if _, err := s.Store.DB.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	for range 64 {
		session, err := s.Authenticate(ctx, token)
		if err != nil || !session.Allows("files") {
			t.Fatal("active authority lost", err)
		}
	}
	if _, err := s.Store.DB.Exec("UPDATE devices SET capabilities='[]' WHERE id=?", actor.Device.ID); err != nil {
		t.Fatal(err)
	}
	session, err := s.Authenticate(ctx, token)
	if err != nil || session.Allows("files") {
		t.Fatal("capability change was cached", err)
	}
	var count, distinct int
	if err := s.Store.DB.QueryRow("SELECT count(*),count(DISTINCT seen) FROM activity_write_audit").Scan(&count, &distinct); err != nil || count < 1 || count != distinct {
		t.Fatal("identical session activity timestamps were rewritten", count, distinct, err)
	}
	if _, err := s.Store.DB.Exec("UPDATE devices SET revoked_at=1 WHERE id=?", actor.Device.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(ctx, token); !errors.Is(err, ErrDenied) {
		t.Fatal("revoked authority was cached", err)
	}
	var after int
	if err := s.Store.DB.QueryRow("SELECT count(*) FROM activity_write_audit").Scan(&after); err != nil || after != count {
		t.Fatal("revoked session activity changed", after, err)
	}
}
