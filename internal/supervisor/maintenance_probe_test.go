package supervisor

import (
	"context"
	"errors"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

type maintenanceProbeBackend struct {
	*fakeBackend
	active  bool
	failure error
	cancel  context.CancelFunc
	probes  int
}

func (b *maintenanceProbeBackend) Running(context.Context, string) (bool, error) {
	b.probes++
	if b.cancel != nil {
		b.cancel()
	}
	return b.active, b.failure
}

func TestRuntimeMaintenanceRequiresObservedDomainExit(t *testing.T) {
	for _, scenario := range []string{"running", "probe-error", "cancel", "invalid-id", "stopped", "removed-running"} {
		t.Run(scenario, func(t *testing.T) {
			m, original := newManager(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			start := startRequest()
			if _, err := m.Apply(ctx, start); err != nil {
				t.Fatal(err)
			}
			if _, err := m.Apply(ctx, Request{Version: 1, OperationID: state.Random(), InstanceID: start.InstanceID, Action: "stop", Revision: 2, PolicyGeneration: 1}); err != nil {
				t.Fatal(err)
			}
			probeError := errors.New("fixture domain probe unavailable")
			b := &maintenanceProbeBackend{fakeBackend: original}
			switch scenario {
			case "running":
				b.active = true
			case "removed-running":
				b.active = true
				if _, err := m.Store.DB.Exec("UPDATE runtime_instances SET state='removed' WHERE id=?", start.InstanceID); err != nil {
					t.Fatal(err)
				}
			case "probe-error":
				b.failure = probeError
			case "cancel":
				b.cancel = cancel
			case "invalid-id":
				if _, err := m.Store.DB.Exec("UPDATE runtime_instances SET id='../invalid' WHERE id=?", start.InstanceID); err != nil {
					t.Fatal(err)
				}
			}
			m.Backend = b
			token, err := m.BeginRuntimeMaintenance(ctx)
			if scenario == "stopped" {
				if err != nil || token == "" || b.probes != 1 {
					t.Fatal(token, err, b.probes)
				}
				if err = m.EndRuntimeMaintenance(ctx, token); err != nil {
					t.Fatal(err)
				}
			} else {
				if err == nil || token != "" {
					t.Fatal("unsafe domain admitted", token, err)
				}
				if scenario == "probe-error" && !errors.Is(err, probeError) {
					t.Fatal("probe error concealed", err)
				}
				if scenario == "cancel" && !errors.Is(err, context.Canceled) {
					t.Fatal("cancellation concealed", err)
				}
				if scenario == "invalid-id" && b.probes != 0 {
					t.Fatal("invalid identifier reached backend", b.probes)
				}
			}
			var count int
			if err := m.Store.DB.QueryRow("SELECT count(*) FROM settings WHERE key=?", runtimeMaintenanceKey).Scan(&count); err != nil || count != 0 {
				t.Fatal("refusal retained barrier", count, err)
			}
		})
	}
}
