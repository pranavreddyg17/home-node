package state

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"
)

func TestMaintenanceBarrierSurvivesReopenAndRequiresOwner(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "state")
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	token, err := s.BeginMaintenance(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.BeginMaintenance(ctx); !errors.Is(err, ErrMaintenance) {
		t.Fatal("overlapping maintenance accepted", err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.Transaction(ctx, RequireAdmission); !errors.Is(err, ErrMaintenance) {
		t.Fatal("restart reopened admission", err)
	}
	if err = s.EndMaintenance(ctx, Random()); !errors.Is(err, ErrMaintenanceOwner) {
		t.Fatal("foreign owner released barrier", err)
	}
	if err = s.EndMaintenance(ctx, token); err != nil {
		t.Fatal(err)
	}
	if err = s.Transaction(ctx, RequireAdmission); err != nil {
		t.Fatal(err)
	}
	next, err := s.BeginMaintenance(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.EndMaintenance(ctx, token); !errors.Is(err, ErrMaintenanceOwner) {
		t.Fatal("old token released successor", err)
	}
	if err = s.EndMaintenance(ctx, next); err != nil {
		t.Fatal(err)
	}
}

func TestMaintenanceAcquisitionSerializesWithAdmission(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	entered := make(chan struct{})
	release := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := s.Transaction(ctx, func(tx *sql.Tx) error {
			if err := RequireAdmission(tx); err != nil {
				return err
			}
			close(entered)
			<-release
			_, err := tx.Exec("INSERT INTO settings VALUES('admitted.fixture','intent')")
			return err
		})
		if err != nil {
			t.Error(err)
		}
	}()
	<-entered
	acquired := make(chan string, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		token, err := s.BeginMaintenance(ctx)
		if err != nil {
			t.Error(err)
		}
		acquired <- token
	}()
	close(release)
	wg.Wait()
	token := <-acquired
	if token == "" {
		t.Fatal("missing barrier")
	}
	if err = s.Transaction(ctx, RequireAdmission); !errors.Is(err, ErrMaintenance) {
		t.Fatal(err)
	}
	var value string
	if err = s.DB.QueryRow("SELECT value FROM settings WHERE key='admitted.fixture'").Scan(&value); err != nil || value != "intent" {
		t.Fatal("pre-barrier intent lost", value, err)
	}
}
