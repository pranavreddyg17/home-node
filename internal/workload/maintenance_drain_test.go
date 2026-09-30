package workload

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestMaintenanceDrainStopsPersistentAppsThroughWorker(t *testing.T) {
	s, original, device := service(t)
	startFiles(t, s, device)
	if _, err := s.Store.DB.Exec("INSERT INTO apps(workload,instance_id,state,updated_at,revision) VALUES('ai',?,'running',1,1)", state.Random()); err != nil {
		t.Fatal(err)
	}
	b := &appShutdownBackend{testBackend: original}
	s.Backend = b
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	token, err := s.Store.BeginMaintenance(ctx)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- s.DrainMaintenance(ctx, token, device) }()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
			if len(b.requests) != 2 {
				t.Fatal("apps not drained", b.requests)
			}
			for _, request := range b.requests {
				if request.Action != "shutdown" {
					t.Fatal("forced shutdown", request)
				}
			}
			inventory, err := s.Store.InspectMaintenance(ctx, token)
			if err != nil || inventory != (state.MaintenanceInventory{}) {
				t.Fatal(inventory, err)
			}
			if _, err = s.CreateConversation(ctx, "still blocked"); !errors.Is(err, state.ErrMaintenance) {
				t.Fatal("drain released admission", err)
			}
			return
		case <-ctx.Done():
			t.Fatal("drain did not complete", ctx.Err())
		case <-ticker.C:
			if err := s.processAppKind(ctx, "app.stop"); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestMaintenanceDrainCancellationPreservesPendingWorkAndBarrier(t *testing.T) {
	s, original, device := service(t)
	startFiles(t, s, device)
	ctx := context.Background()
	transfer, err := s.CreateTransfer(ctx, device, "pending", 1, sum([]byte("x")))
	if err != nil {
		t.Fatal(err)
	}
	token, err := s.Store.BeginMaintenance(ctx)
	if err != nil {
		t.Fatal(err)
	}
	b := &appShutdownBackend{testBackend: original}
	s.Backend = b
	deadline, cancel := context.WithTimeout(ctx, 80*time.Millisecond)
	defer cancel()
	if err := s.DrainMaintenance(deadline, token, device); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if len(b.requests) != 0 {
		t.Fatal("drain forced incomplete work", b.requests)
	}
	var count int
	if err := s.Store.DB.QueryRow("SELECT count(*) FROM transfers WHERE id=? AND state NOT IN('ready','cancelled','expired')", transfer.ID).Scan(&count); err != nil || count != 1 {
		t.Fatal("pending transfer changed", count, err)
	}
	if _, err := s.Store.InspectMaintenance(ctx, token); err != nil {
		t.Fatal("cancellation released barrier", err)
	}
}

func TestMaintenanceDrainRefusesUncertainStateWithoutRuntimeEffects(t *testing.T) {
	for _, scenario := range []string{"failed-app", "uncertain-operation"} {
		t.Run(scenario, func(t *testing.T) {
			s, original, device := service(t)
			startFiles(t, s, device)
			ctx := context.Background()
			token, err := s.Store.BeginMaintenance(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "failed-app" {
				_, err = s.Store.DB.Exec("UPDATE apps SET state='failed' WHERE workload='files'")
			} else {
				_, err = s.Store.DB.Exec("UPDATE operations SET state='requires-action' WHERE kind='app.start'")
			}
			if err != nil {
				t.Fatal(err)
			}
			b := &appShutdownBackend{testBackend: original}
			s.Backend = b
			if err = s.DrainMaintenance(ctx, token, device); !errors.Is(err, ErrConflict) {
				t.Fatal(err)
			}
			if len(b.requests) != 0 {
				t.Fatal("uncertainty reached runtime", b.requests)
			}
			if _, err = s.Store.InspectMaintenance(ctx, token); err != nil {
				t.Fatal("uncertainty released barrier", err)
			}
		})
	}
}
