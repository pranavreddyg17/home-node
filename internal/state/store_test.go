package state

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCancelledTransactionPreservesCauseAfterRollback(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err = s.Transaction(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("INSERT INTO settings VALUES('cancelled','private')"); err != nil {
			return err
		}
		// Establish the same completed-rollback state database/sql reaches when
		// cancellation wins the race with Commit, without relying on scheduling.
		if err := tx.Rollback(); err != nil {
			return err
		}
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) || !errors.Is(err, sql.ErrTxDone) {
		t.Fatal("cancellation or original database failure lost", err)
	}
	var count int
	if err := s.DB.QueryRow("SELECT count(*) FROM settings WHERE key='cancelled'").Scan(&count); err != nil || count != 0 {
		t.Fatal("cancelled write survived rollback", count, err)
	}
}

func TestRollbackAndReopen(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("interrupt")
	err = s.Transaction(context.Background(), func(tx *sql.Tx) error {
		if _, err := tx.Exec("INSERT INTO settings VALUES('partial','value')"); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var count int
	if err = s.DB.QueryRow("SELECT count(*) FROM settings").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("rolled back write survived")
	}
}
func TestRefusesExposedStateAndSymlink(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "state")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); err == nil {
		t.Fatal("world-readable state accepted")
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(link); err == nil {
		t.Fatal("symlink state accepted")
	}
}
func TestRejectsFutureSchema(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec("PRAGMA user_version=999"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if s, err = Open(dir); err == nil {
		s.Close()
		t.Fatal("future schema silently downgraded")
	}
}
