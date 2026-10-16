package workload

import (
	"context"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

func TestReconcileRejectsUnboundObservationWithoutClearingStopFence(t *testing.T) {
	for _, scenario := range []string{"id", "workload", "revision-zero", "revision-overflow", "state"} {
		t.Run(scenario, func(t *testing.T) {
			s, original, device := service(t)
			startFiles(t, s, device)
			ctx := context.Background()
			op, err := s.AppAction(ctx, device, state.Random(), "files", "stop")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.Store.DB.Exec("UPDATE operations SET state='requires-action' WHERE id=?", op.ID); err != nil {
				t.Fatal(err)
			}
			s.Backend = &alteredAppReplyBackend{testBackend: original, alter: func(i supervisor.Instance) supervisor.Instance {
				switch scenario {
				case "id":
					i.ID = state.Random()
				case "workload":
					i.Workload = "ai"
				case "revision-zero":
					i.Revision = 0
				case "revision-overflow":
					i.Revision = 1<<53 + 1
				case "state":
					i.State = "unknown"
				}
				return i
			}}
			if err = s.Reconcile(ctx); err != nil {
				t.Fatal(err)
			}
			apps, err := s.Apps(ctx)
			if err != nil || len(apps) != 1 || apps[0].State != "stopping" || apps[0].Revision != 2 {
				t.Fatal("invalid inspection cleared stop fence", apps, err)
			}
		})
	}
}

func TestReconcileDoesNotOverwriteIntentChangedDuringInspection(t *testing.T) {
	s, original, device := service(t)
	startFiles(t, s, device)
	ctx := context.Background()
	var newer Operation
	s.Backend = &alteredAppReplyBackend{testBackend: original, alter: func(i supervisor.Instance) supervisor.Instance {
		var err error
		newer, err = s.AppAction(ctx, device, state.Random(), "files", "stop")
		if err != nil {
			t.Fatal(err)
		}
		return i
	}}
	if err := s.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	apps, err := s.Apps(ctx)
	if err != nil || len(apps) != 1 || apps[0].State != "stopping" || apps[0].Revision != 2 {
		t.Fatal("stale inspection overwrote new stop", apps, err)
	}
	var current string
	if err = s.Store.DB.QueryRow("SELECT operation_id FROM apps WHERE workload='files'").Scan(&current); err != nil || current != newer.ID {
		t.Fatal("new operation ownership lost", current, newer, err)
	}
}

func TestReconcileSettlesUncertainOperationOnlyAfterVerifiedStop(t *testing.T) {
	for _, running := range []bool{false, true} {
		s, backend, device := service(t)
		startFiles(t, s, device)
		ctx := context.Background()
		key := state.Random()
		op, err := s.AppAction(ctx, device, key, "files", "stop")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.Store.DB.Exec("UPDATE operations SET state='requires-action' WHERE id=?", op.ID); err != nil {
			t.Fatal(err)
		}
		backend.running = running
		if err = s.Reconcile(ctx); err != nil {
			t.Fatal(err)
		}
		observed, err := s.Operation(ctx, device, op.ID)
		if err != nil {
			t.Fatal(err)
		}
		expected := "interrupted"
		if running {
			expected = "requires-action"
		}
		if observed.State != expected || string(observed.Result) != string(op.Result) {
			t.Fatal("recovery lost uncertainty or original intent", observed, op)
		}
		apps, err := s.Apps(ctx)
		if err != nil || len(apps) != 1 || apps[0].RequiresAttention != running {
			t.Fatal(apps, err)
		}
		replay, err := s.AppAction(ctx, device, key, "files", "stop")
		if err != nil || replay.ID != op.ID || replay.State != expected {
			t.Fatal("replay lost recovered operation", replay, err)
		}
		if !running {
			if _, err = s.AppAction(ctx, device, state.Random(), "files", "start"); err != nil {
				t.Fatal("verified stop left start blocked", err)
			}
		}
	}
}

func TestReconcileStopSettlementRollsBackTogether(t *testing.T) {
	s, backend, device := service(t)
	startFiles(t, s, device)
	ctx := context.Background()
	op, err := s.AppAction(ctx, device, state.Random(), "files", "stop")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Store.DB.Exec("UPDATE operations SET state='requires-action' WHERE id=?", op.ID); err != nil {
		t.Fatal(err)
	}
	backend.running = false
	if _, err = s.Store.DB.Exec(`CREATE TRIGGER fixture_settlement BEFORE UPDATE ON operations WHEN NEW.state='interrupted' BEGIN SELECT RAISE(ABORT,'fixture settlement failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err = s.Reconcile(ctx); err == nil {
		t.Fatal("settlement failure hidden")
	}
	apps, err := s.Apps(ctx)
	if err != nil || len(apps) != 1 || apps[0].State != "stopping" || !apps[0].RequiresAttention {
		t.Fatal("partial recovery committed", apps, err)
	}
}
