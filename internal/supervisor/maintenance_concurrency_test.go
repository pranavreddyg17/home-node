package supervisor

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/state"
)

type heldStopBackend struct {
	*fakeBackend
	entered chan struct{}
	release chan struct{}
}

func (b *heldStopBackend) Stop(ctx context.Context, id string) error {
	close(b.entered)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-b.release:
		return b.fakeBackend.Stop(ctx, id)
	}
}

func TestRuntimeMaintenanceWaitsForAdmittedBackendCall(t *testing.T) {
	for _, cancelWait := range []bool{false, true} {
		t.Run(map[bool]string{false: "wait", true: "cancel"}[cancelWait], func(t *testing.T) {
			m, original := newManager(t)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			start := startRequest()
			if _, err := m.Apply(ctx, start); err != nil {
				t.Fatal(err)
			}
			b := &heldStopBackend{fakeBackend: original, entered: make(chan struct{}), release: make(chan struct{})}
			m.Backend = b
			var once sync.Once
			release := func() { once.Do(func() { close(b.release) }) }
			defer release()
			stopped := make(chan error, 1)
			go func() {
				_, err := m.Apply(ctx, Request{Version: 1, OperationID: state.Random(), InstanceID: start.InstanceID, Action: "stop", Revision: 2, PolicyGeneration: 1})
				stopped <- err
			}()
			select {
			case <-b.entered:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			waitCtx := ctx
			if cancelWait {
				var cancelShort context.CancelFunc
				waitCtx, cancelShort = context.WithTimeout(ctx, 80*time.Millisecond)
				defer cancelShort()
			}
			type result struct {
				token string
				err   error
			}
			acquired := make(chan result, 1)
			go func() { token, err := m.BeginRuntimeMaintenance(waitCtx); acquired <- result{token, err} }()
			if cancelWait {
				select {
				case r := <-acquired:
					if !errors.Is(r.err, context.DeadlineExceeded) || r.token != "" {
						t.Fatal("wait ignored cancellation", r)
					}
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
			} else {
				select {
				case r := <-acquired:
					t.Fatal("barrier acquired during external stop", r)
				case <-time.After(80 * time.Millisecond):
				}
			}
			release()
			select {
			case err := <-stopped:
				if err != nil {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if cancelWait {
				var count int
				if err := m.Store.DB.QueryRow("SELECT count(*) FROM settings WHERE key=?", runtimeMaintenanceKey).Scan(&count); err != nil || count != 0 {
					t.Fatal("canceled acquisition persisted", count, err)
				}
				token, err := m.BeginRuntimeMaintenance(ctx)
				if err != nil {
					t.Fatal("canceled waiter retained lock", err)
				}
				if err = m.EndRuntimeMaintenance(ctx, token); err != nil {
					t.Fatal(err)
				}
			} else {
				select {
				case r := <-acquired:
					if r.err != nil || r.token == "" {
						t.Fatal(r)
					}
					if err := m.EndRuntimeMaintenance(ctx, r.token); err != nil {
						t.Fatal(err)
					}
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
			}
		})
	}
}

type heldAuditBackend struct {
	*fakeBackend
	entered chan struct{}
	release chan struct{}
}

func (b *heldAuditBackend) ValidateHost(ctx context.Context, _ Policy) error {
	close(b.entered)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-b.release:
		return nil
	}
}

func TestRuntimeMaintenanceWaitsForAuditCapturedBeforeStop(t *testing.T) {
	m, original := newManager(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := startRequest()
	if _, err := m.Apply(ctx, start); err != nil {
		t.Fatal(err)
	}
	b := &heldAuditBackend{fakeBackend: original, entered: make(chan struct{}), release: make(chan struct{})}
	m.Backend = b
	var once sync.Once
	release := func() { once.Do(func() { close(b.release) }) }
	defer release()
	audited := make(chan error, 1)
	go func() { audited <- m.Audit(ctx) }()
	select {
	case <-b.entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	// A launch arriving while audit retains an old runtime observation must
	// wait rather than enter backend preparation before audit teardown finishes.
	startDeadline, cancelStart := context.WithTimeout(ctx, 80*time.Millisecond)
	replacement := startRequest()
	replacement.OperationID = state.Random()
	replacement.InstanceID = state.Random()
	_, startErr := m.Apply(startDeadline, replacement)
	cancelStart()
	if !errors.Is(startErr, context.DeadlineExceeded) {
		t.Fatal("launch crossed retained audit observation", startErr)
	}
	if _, err := m.Apply(ctx, Request{Version: 1, OperationID: state.Random(), InstanceID: start.InstanceID, Action: "stop", Revision: 2, PolicyGeneration: 1}); err != nil {
		t.Fatal("audit serialized emergency-compatible mutation", err)
	}
	deadline, cancelWait := context.WithTimeout(ctx, 80*time.Millisecond)
	defer cancelWait()
	if _, err := m.BeginRuntimeMaintenance(deadline); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("barrier crossed in-flight audit", err)
	}
	release()
	select {
	case err := <-audited:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	token, err := m.BeginRuntimeMaintenance(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = m.EndRuntimeMaintenance(ctx, token); err != nil {
		t.Fatal(err)
	}
}
