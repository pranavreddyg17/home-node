package supervisor

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type heldPreparationBackend struct {
	*fakeBackend
	entered chan struct{}
	release chan struct{}
}

func (b *heldPreparationBackend) Prepare(ctx context.Context, d Domain) error {
	close(b.entered)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-b.release:
		return nil
	}
}

func TestReconciliationWaitsForActivePreparation(t *testing.T) {
	m, original := newManager(t)
	b := &heldPreparationBackend{fakeBackend: original, entered: make(chan struct{}), release: make(chan struct{})}
	m.Backend = b
	var once sync.Once
	release := func() { once.Do(func() { close(b.release) }) }
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r := startRequest()
	done := make(chan error, 1)
	go func() { _, err := m.Apply(ctx, r); done <- err }()
	select {
	case <-b.entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	short, cancelShort := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancelShort()
	if err := m.Reconcile(short); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("recovery raced preparation", err)
	}
	if original.stops != 0 {
		t.Fatal("recovery stopped active preparation", original.stops)
	}
	release()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err := m.Reconcile(ctx); err != nil {
		t.Fatal("recovery after preparation", err)
	}
	i, err := m.Inspect(ctx, r.InstanceID)
	if err != nil || i.State != "interrupted" || original.stops != 1 {
		t.Fatal("recovery failed to stop completed launch", i, err, original.stops)
	}
}
