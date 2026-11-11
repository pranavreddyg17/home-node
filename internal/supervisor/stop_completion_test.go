package supervisor

import (
	"context"
	"errors"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

type changedStopBackend struct {
	*fakeBackend
	change func()
}

func (b *changedStopBackend) Stop(ctx context.Context, id string) error {
	if err := b.fakeBackend.Stop(ctx, id); err != nil {
		return err
	}
	b.change()
	return nil
}

func TestForcedStopCompletionPreservesNewerOrInterruptedOwnership(t *testing.T) {
	for _, scenario := range []string{"revision", "phase", "operation", "request", "removed"} {
		t.Run(scenario, func(t *testing.T) {
			m, b := newManager(t)
			ctx := context.Background()
			start := startRequest()
			if _, err := m.Apply(ctx, start); err != nil {
				t.Fatal(err)
			}
			stop := Request{Version: 1, OperationID: state.Random(), InstanceID: start.InstanceID, Action: "stop", Revision: 2, PolicyGeneration: 1}
			m.Backend = &changedStopBackend{fakeBackend: b, change: func() {
				var err error
				switch scenario {
				case "removed":
					_, err = m.Store.DB.Exec("DELETE FROM runtime_instances WHERE id=?", start.InstanceID)
				case "revision":
					_, err = m.Store.DB.Exec("UPDATE runtime_instances SET revision=3,state='stopping' WHERE id=?", start.InstanceID)
				case "phase":
					_, err = m.Store.DB.Exec("UPDATE runtime_instances SET state='interrupted' WHERE id=?", start.InstanceID)
				case "request":
					_, err = m.Store.DB.Exec("UPDATE runtime_operations SET request_hash='changed' WHERE id=?", stop.OperationID)
				case "operation":
					_, err = m.Store.DB.Exec("UPDATE runtime_operations SET state='interrupted' WHERE id=?", stop.OperationID)
				}
				if err != nil {
					t.Fatal(err)
				}
			}}
			if _, err := m.Apply(ctx, stop); !errors.Is(err, ErrPolicy) {
				t.Fatal("stale stop completion accepted", err)
			}
			instance, err := m.Inspect(ctx, start.InstanceID)
			if scenario != "removed" && (err != nil || instance.State == "stopped") {
				t.Fatal("stale instance completion committed", instance, err)
			}
			var phase string
			if err = m.Store.DB.QueryRow("SELECT state FROM runtime_operations WHERE id=?", stop.OperationID).Scan(&phase); err != nil || phase == "succeeded" {
				t.Fatal("stale operation completion committed", phase, err)
			}
		})
	}
}

func TestStopReplayPreservesRemovedVideo(t *testing.T) {
	m, b := newManager(t)
	ctx := context.Background()
	start := startRequest()
	start.Workload = "video"
	m.Manifest.Images[0].ID = "video"
	if _, err := m.Apply(ctx, start); err != nil {
		t.Fatal(err)
	}
	stop := Request{Version: 1, OperationID: state.Random(), InstanceID: start.InstanceID, Action: "stop", Revision: 2, PolicyGeneration: 1}
	if _, err := m.Apply(ctx, stop); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Store.DB.Exec("UPDATE runtime_instances SET state='removed',revision=3 WHERE id=?", start.InstanceID); err != nil {
		t.Fatal(err)
	}
	calls := b.stops
	got, err := m.Apply(ctx, stop)
	if err != nil || got.State != "removed" || got.Revision != 3 || b.stops != calls {
		t.Fatal("stop replay modified retired instance", got, err)
	}
	stop.OperationID = state.Random()
	stop.Revision = 4
	if _, err := m.Apply(ctx, stop); !errors.Is(err, ErrPolicy) {
		t.Fatal("new stop admitted for retired instance", err)
	}
}
