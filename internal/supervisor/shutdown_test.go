package supervisor

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestShutdownWaitsForObservedExit(t *testing.T) {
	probes, requests := 0, 0
	err := awaitShutdown(context.Background(), state.Random(), func(context.Context, string) (bool, error) { probes++; return probes < 4, nil }, func(context.Context, string) error { requests++; return nil }, time.Millisecond)
	if err != nil || probes != 4 || requests != 1 {
		t.Fatal("shutdown acknowledgement treated as exit", probes, requests, err)
	}
}

func TestShutdownDoesNotForceStopOnCancellationOrFailure(t *testing.T) {
	for _, scenario := range []string{"already-stopped", "probe-failure", "request-failure", "cancelled", "invalid-id"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			requests := 0
			sentinel := errors.New("fixture failure")
			id := state.Random()
			if scenario == "invalid-id" {
				id = "../foreign"
			}
			err := awaitShutdown(ctx, id, func(context.Context, string) (bool, error) {
				if scenario == "probe-failure" {
					return false, sentinel
				}
				return scenario != "already-stopped", nil
			}, func(context.Context, string) error {
				requests++
				if scenario == "request-failure" {
					return sentinel
				}
				cancel()
				return nil
			}, time.Millisecond)
			switch scenario {
			case "already-stopped":
				if err != nil || requests != 0 {
					t.Fatal(requests, err)
				}
			case "probe-failure":
				if !errors.Is(err, sentinel) || requests != 0 {
					t.Fatal(requests, err)
				}
			case "request-failure":
				if !errors.Is(err, sentinel) || requests != 1 {
					t.Fatal(requests, err)
				}
			case "cancelled":
				if !errors.Is(err, context.Canceled) || requests != 1 {
					t.Fatal(requests, err)
				}
			case "invalid-id":
				if !errors.Is(err, ErrPolicy) || requests != 0 {
					t.Fatal(requests, err)
				}
			}
		})
	}
}

func TestShutdownDeadlineWhileGuestIgnoresPoweroff(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	requests := 0
	err := awaitShutdown(ctx, state.Random(), func(context.Context, string) (bool, error) { return true, nil }, func(context.Context, string) error { requests++; return nil }, time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) || requests > 1 {
		t.Fatal("ignored poweroff did not honor deadline", requests, err)
	}
}

func TestShutdownAuditDeadlineAndEmergencyEnforcement(t *testing.T) {
	for _, scenario := range []string{"waiting", "expired", "missing", "host-failure", "isolation-failure", "recovery"} {
		t.Run(scenario, func(t *testing.T) {
			m, b := newManager(t)
			ctx := context.Background()
			id := state.Random()
			if _, err := m.Apply(ctx, Request{Version: 1, OperationID: state.Random(), Action: "start", Workload: "files", InstanceID: id, PolicyGeneration: 1, Revision: 1}); err != nil {
				t.Fatal(err)
			}
			if _, err := m.Store.DB.Exec("UPDATE runtime_instances SET state='shutting-down',desired='stopped' WHERE id=?", id); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Unix() + 60
			if scenario == "expired" {
				deadline = time.Now().Unix() - 1
			}
			if scenario != "missing" {
				if _, err := m.Store.DB.Exec("INSERT INTO settings VALUES(?,?)", shutdownDeadlineKey(id), strconv.FormatInt(deadline, 10)); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "host-failure" {
				b.hostErr = errors.New("fixture host policy failure")
			}
			if scenario == "isolation-failure" {
				b.verifyErr = errors.New("fixture isolation failure")
			}
			if scenario == "recovery" {
				if err := m.Reconcile(ctx); err != nil {
					t.Fatal(err)
				}
			} else if err := m.Audit(ctx); err != nil {
				t.Fatal(err)
			}
			instance, err := m.Inspect(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "waiting" {
				if b.stops != 0 || instance.State != "shutting-down" {
					t.Fatal("audit interrupted valid shutdown wait", b.stops, instance)
				}
			} else if b.stops != 1 || instance.State != "interrupted" {
				t.Fatal("audit missed emergency shutdown enforcement", b.stops, instance)
			}
		})
	}
}

type shutdownFixtureBackend struct {
	*fakeBackend
	shutdowns int
	failure   error
	during    func()
}

func (b *shutdownFixtureBackend) Shutdown(context.Context, string) error {
	b.shutdowns++
	if b.during != nil {
		b.during()
	}
	if b.failure != nil {
		return b.failure
	}
	b.running = false
	return nil
}

func TestJournaledShutdownSuccessReplayAndInterveningTeardown(t *testing.T) {
	for _, scenario := range []string{"success", "failure", "intervening-stop", "deadline-changed", "audit-interruption"} {
		t.Run(scenario, func(t *testing.T) {
			m, original := newManager(t)
			b := &shutdownFixtureBackend{fakeBackend: original}
			m.Backend = b
			ctx := context.Background()
			id := state.Random()
			if _, err := m.Apply(ctx, Request{Version: 1, OperationID: state.Random(), Action: "start", Workload: "files", InstanceID: id, PolicyGeneration: 1, Revision: 1}); err != nil {
				t.Fatal(err)
			}
			request := Request{Version: 1, OperationID: state.Random(), Action: "shutdown", InstanceID: id, PolicyGeneration: 1, Revision: 2}
			b.during = func() {
				instance, err := m.Inspect(ctx, id)
				if err != nil || instance.State != "shutting-down" {
					t.Fatal("runtime call preceded journal", instance, err)
				}
				if scenario == "audit-interruption" {
					b.hostErr = errors.New("fixture host failure during shutdown")
					if err = m.Audit(ctx); err != nil {
						t.Fatal(err)
					}
				}
				if scenario == "deadline-changed" {
					if _, err = m.Store.DB.Exec("UPDATE settings SET value='9999999999' WHERE key=?", shutdownDeadlineKey(id)); err != nil {
						t.Fatal(err)
					}
				}
				if scenario == "intervening-stop" {
					if _, err = m.Apply(ctx, Request{Version: 1, OperationID: state.Random(), Action: "stop", InstanceID: id, PolicyGeneration: 1, Revision: 3}); err != nil {
						t.Fatal(err)
					}
				}
			}
			if scenario == "failure" {
				b.failure = errors.New("fixture shutdown refusal")
			}
			result, err := m.Apply(ctx, request)
			if scenario == "success" {
				if err != nil || result.State != "stopped" || b.stops != 0 {
					t.Fatal(result, err)
				}
				if _, err = m.Apply(ctx, request); err != nil || b.shutdowns != 1 {
					t.Fatal("replay repeated shutdown", b.shutdowns, err)
				}
			} else {
				if err == nil {
					t.Fatal("failed/intervened shutdown reported success")
				}
				var phase string
				if err = m.Store.DB.QueryRow("SELECT state FROM runtime_operations WHERE id=?", request.OperationID).Scan(&phase); err != nil || phase != "pending" {
					t.Fatal("shutdown authority marked complete", phase, err)
				}
			}
		})
	}
}

func TestShutdownAdmissionRejectsInvalidAuthorityWithoutJournal(t *testing.T) {
	for _, scenario := range []string{"unsupported-backend", "wrong-workload", "video-instance", "stale-revision", "wrong-policy"} {
		t.Run(scenario, func(t *testing.T) {
			m, original := newManager(t)
			ctx := context.Background()
			id := state.Random()
			if _, err := m.Apply(ctx, Request{Version: 1, OperationID: state.Random(), Action: "start", Workload: "files", InstanceID: id, PolicyGeneration: 1, Revision: 1}); err != nil {
				t.Fatal(err)
			}
			b := &shutdownFixtureBackend{fakeBackend: original}
			if scenario != "unsupported-backend" {
				m.Backend = b
			}
			request := Request{Version: 1, OperationID: state.Random(), Action: "shutdown", InstanceID: id, PolicyGeneration: 1, Revision: 2}
			switch scenario {
			case "wrong-workload":
				request.Workload = "ai"
			case "video-instance":
				if _, err := m.Store.DB.Exec("UPDATE runtime_instances SET workload='video' WHERE id=?", id); err != nil {
					t.Fatal(err)
				}
			case "stale-revision":
				request.Revision = 1
			case "wrong-policy":
				request.PolicyGeneration = 2
			}
			if _, err := m.Apply(ctx, request); !errors.Is(err, ErrPolicy) {
				t.Fatal("invalid shutdown admitted", err)
			}
			if b.shutdowns != 0 || b.stops != 0 {
				t.Fatal("denial issued runtime effects", b.shutdowns, b.stops)
			}
			var count int
			if err := m.Store.DB.QueryRow("SELECT count(*) FROM runtime_operations WHERE id=?", request.OperationID).Scan(&count); err != nil || count != 0 {
				t.Fatal("denial retained operation", count, err)
			}
			if err := m.Store.DB.QueryRow("SELECT count(*) FROM settings WHERE key IN(?,?)", shutdownOwnerKey(id), shutdownDeadlineKey(id)).Scan(&count); err != nil || count != 0 {
				t.Fatal("denial retained shutdown authority", count, err)
			}
			instance, err := m.Inspect(ctx, id)
			if err != nil || instance.State != "running" || instance.Revision != 1 {
				t.Fatal("denial changed runtime intent", instance, err)
			}
		})
	}
}
