package workload

import (
	"context"
	"errors"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

type uncertainStartBackend struct {
	*testBackend
	scenario string
	requests []supervisor.Request
}

func (b *uncertainStartBackend) Apply(ctx context.Context, r supervisor.Request) (supervisor.Instance, error) {
	b.requests = append(b.requests, r)
	if r.Action == "stop" && b.scenario == "cleanup-fails" {
		return supervisor.Instance{}, errors.New("fixture cleanup unavailable")
	}
	instance, err := b.testBackend.Apply(ctx, r)
	if err != nil {
		return instance, err
	}
	if r.Action == "start" {
		if b.scenario == "wrong-reply" {
			instance.ID = state.Random()
			return instance, nil
		}
		return supervisor.Instance{}, errors.New("fixture response lost after start")
	}
	return instance, nil
}
func TestUncertainStartupTeardownUsesOnlyIntendedInstanceAndRevision(t *testing.T) {
	for _, scenario := range []string{"lost-reply", "wrong-reply", "cleanup-fails"} {
		t.Run(scenario, func(t *testing.T) {
			s, original, device := service(t)
			b := &uncertainStartBackend{testBackend: original, scenario: scenario}
			s.Backend = b
			ctx := context.Background()
			op, err := s.AppAction(ctx, device, state.Random(), "files", "start")
			if err != nil {
				t.Fatal(err)
			}
			if err = s.processApp(ctx); err != nil {
				t.Fatal(err)
			}
			if len(b.requests) != 2 || b.requests[1].Action != "stop" || b.requests[1].InstanceID != b.requests[0].InstanceID || b.requests[1].Revision != b.requests[0].Revision {
				t.Fatal("cleanup used untrusted reply identity", b.requests)
			}
			result, err := s.Operation(ctx, device, op.ID)
			if err != nil {
				t.Fatal(err)
			}
			expected := "failed"
			if scenario == "cleanup-fails" {
				expected = "requires-action"
			}
			if result.State != expected {
				t.Fatal("uncertain teardown misreported", result)
			}
			if err = s.processApp(ctx); err != nil || len(b.requests) != 2 {
				t.Fatal("uncertain start retried automatically", b.requests, err)
			}
		})
	}
}
