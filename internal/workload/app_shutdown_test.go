package workload

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

type appShutdownBackend struct {
	*testBackend
	requests []supervisor.Request
	refuse   bool
}

func (b *appShutdownBackend) Apply(ctx context.Context, r supervisor.Request) (supervisor.Instance, error) {
	b.requests = append(b.requests, r)
	if r.Action == "shutdown" && b.refuse {
		return supervisor.Instance{}, errors.New("fixture shutdown refusal")
	}
	return b.testBackend.Apply(ctx, r)
}

func TestRunningAppStopPersistsCooperativeIntentWithoutForcedFallback(t *testing.T) {
	for _, refuse := range []bool{false, true} {
		s, original, device := service(t)
		startFiles(t, s, device)
		b := &appShutdownBackend{testBackend: original, refuse: refuse}
		s.Backend = b
		ctx := context.Background()
		key := state.Random()
		operation, err := s.AppAction(ctx, device, key, "files", "stop")
		if err != nil {
			t.Fatal(err)
		}
		var intent appIntent
		if err = json.Unmarshal(operation.Result, &intent); err != nil || !intent.CooperativeStop || len(b.requests) != 0 {
			t.Fatal("shutdown mode not journaled before effect", intent, err)
		}
		if err = s.processApp(ctx); err != nil {
			t.Fatal(err)
		}
		if len(b.requests) != 1 || b.requests[0].Action != "shutdown" || b.requests[0].OperationID != operation.ID || b.requests[0].Revision != intent.Revision {
			t.Fatal("stop escalated or changed intent", b.requests)
		}
		result, err := s.Operation(ctx, device, operation.ID)
		if err != nil {
			t.Fatal(err)
		}
		expected := "succeeded"
		if refuse {
			expected = "failed"
		}
		if result.State != expected {
			t.Fatal("incorrect shutdown result", result)
		}
		if _, err = s.AppAction(ctx, device, key, "files", "stop"); err != nil {
			t.Fatal(err)
		}
		if err = s.processApp(ctx); err != nil || len(b.requests) != 1 {
			t.Fatal("retry repeated runtime effects", b.requests, err)
		}
	}
}
