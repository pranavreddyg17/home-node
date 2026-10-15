package workload

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestTransferCreationReplaySurvivesReopenAndConcurrentRetries(t *testing.T) {
	s, backend, device := service(t)
	ctx := context.Background()
	startFiles(t, s, device)
	key := state.Random()
	first, err := s.CreateTransferWithKey(ctx, device, "original.txt", 19, sum(nil), key)
	if err != nil {
		t.Fatal(err)
	}
	var databasePath string
	if err := s.Store.DB.QueryRow("SELECT file FROM pragma_database_list WHERE name='main'").Scan(&databasePath); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := state.Open(filepath.Dir(databasePath))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	// Replaying an admitted reservation is observation, even while the guest is
	// unavailable and maintenance prevents new work.
	s = New(store, nil, 1)
	token, err := store.BeginMaintenance(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var retries sync.WaitGroup
	for range 8 {
		retries.Add(1)
		go func() {
			defer retries.Done()
			item, err := s.CreateTransferWithKey(ctx, device, "original.txt", 19, sum(nil), key)
			if err != nil || item != first {
				t.Errorf("creation replay changed reservation: %+v %v", item, err)
			}
		}()
	}
	retries.Wait()
	if _, err := s.CreateTransferWithKey(ctx, device, "changed.txt", 19, sum(nil), key); !errors.Is(err, ErrConflict) {
		t.Fatal("key admitted a different request", err)
	}
	var count int
	if err := store.DB.QueryRow("SELECT count(*) FROM transfers").Scan(&count); err != nil || count != 1 {
		t.Fatal("creation duplicated", count, err)
	}
	usage, err := s.FilesStorage(ctx)
	if err != nil || usage.UploadReservedBytes != 19 {
		t.Fatal("quota reserved more than once", usage, err)
	}
	if err := store.EndMaintenance(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CancelTransfer(ctx, device, first.ID); err != nil {
		t.Fatal(err)
	}
	item, err := s.CreateTransferWithKey(ctx, device, "original.txt", 19, sum(nil), key)
	if err != nil || item.ID != first.ID || item.State != "cancelling" {
		t.Fatal("replay resurrected cancelled transfer", item, err)
	}
	s.Backend = backend
	if _, err := s.CreateTransferWithKey(ctx, device, "invalid.txt", 1, sum(nil), "short"); !errors.Is(err, ErrInvalid) {
		t.Fatal("invalid idempotency key", err)
	}
}
