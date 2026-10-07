package install

import (
	"context"
	"errors"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
	"testing"
	"time"
)

func TestInstalledGuestUIDPlanRefusesContendedInstaller(t *testing.T) {
	engine := &Engine{}
	engine.mu.Lock()
	defer engine.mu.Unlock()
	done := make(chan error, 1)
	go func() {
		plan, err := engine.planInstalledGuestUIDProvisioning(context.Background(), supervisor.GuestUIDPool{First: 200000, Last: 200001})
		if plan.OwnerID != "" {
			done <- errors.New("busy installer emitted proposal")
			return
		}
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, ErrConflict) {
			t.Fatal("busy installer not refused", err)
		}
	case <-time.After(time.Second):
		t.Fatal("planning blocked on installer mutex")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := engine.planInstalledGuestUIDProvisioning(ctx, supervisor.GuestUIDPool{}); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost", err)
	}
}
