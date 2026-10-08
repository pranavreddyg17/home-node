package supervisor

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCancelledStartLockWaiterReleasesRuntimeAdmission(t *testing.T) {
	for _, action := range []string{"start", "shutdown"} {
		t.Run(action, func(t *testing.T) {
			m, backend := newManager(t)
			m.startMu.Lock()
			locked := true
			defer func() {
				if locked {
					m.startMu.Unlock()
				}
			}()
			ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
			defer cancel()
			done := make(chan error, 1)
			go func() {
				if action == "shutdown" {
					done <- m.Shutdown(ctx)
					return
				}
				_, err := m.Apply(ctx, startRequest())
				done <- err
			}()
			select {
			case err := <-done:
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatal("wait ignored deadline", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("cancelled waiter retained admission")
			}
			maintenanceCtx, maintenanceCancel := context.WithTimeout(context.Background(), time.Second)
			defer maintenanceCancel()
			unlock, err := m.lockRuntime(maintenanceCtx, true)
			if err != nil {
				t.Fatal("waiter retained runtime reader", err)
			}
			unlock()
			if m.shuttingDown || backend.starts != 0 || backend.stops != 0 {
				t.Fatal("cancelled wait caused effects")
			}
			var instances int
			if err := m.Store.DB.QueryRow(`SELECT count(*) FROM runtime_instances`).Scan(&instances); err != nil || instances != 0 {
				t.Fatal("cancelled wait created inventory", instances, err)
			}
			m.startMu.Unlock()
			locked = false
			if _, err := m.Apply(context.Background(), startRequest()); err != nil {
				t.Fatal("cancelled waiter poisoned future start", err)
			}
		})
	}
}
