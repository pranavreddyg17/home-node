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

func TestQueuedShutdownClosesFutureStartsAfterPreparationRelease(t *testing.T) {
	m, backend := newManager(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := m.Apply(ctx, startRequest()); err != nil {
		t.Fatal(err)
	}
	m.startMu.Lock()
	locked := true
	defer func() {
		if locked {
			m.startMu.Unlock()
		}
	}()
	done := make(chan error, 1)
	go func() { done <- m.Shutdown(ctx) }()
	select {
	case err := <-done:
		t.Fatal("shutdown bypassed preparation lock", err)
	case <-time.After(80 * time.Millisecond):
	}
	m.startMu.Unlock()
	locked = false
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if !m.shuttingDown || backend.running || backend.stops != 1 {
		t.Fatal("queued shutdown did not reconcile")
	}
	if _, err := m.Apply(ctx, startRequest()); !errors.Is(err, ErrPolicy) {
		t.Fatal("shutdown admitted later start", err)
	}
	if backend.starts != 1 {
		t.Fatal("later start reached backend", backend.starts)
	}
	var active int
	if err := m.Store.DB.QueryRow(`SELECT count(*) FROM runtime_instances WHERE desired='running' OR state IN('preparing','running','stopping','shutting-down')`).Scan(&active); err != nil || active != 0 {
		t.Fatal("shutdown retained active inventory", active, err)
	}
}
