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
