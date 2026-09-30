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

type alteredAppReplyBackend struct {
	*testBackend
	alter func(supervisor.Instance) supervisor.Instance
}

func (b *alteredAppReplyBackend) Apply(ctx context.Context, r supervisor.Request) (supervisor.Instance, error) {
	instance, err := b.testBackend.Apply(ctx, r)
	if err != nil {
		return instance, err
	}
	return b.alter(instance), nil
}

func TestAppStopRejectsMismatchedReplyAndStaleCompletion(t *testing.T) {
	for _, scenario := range []string{"id", "revision", "state", "workload", "empty-workload", "superseded", "operation-interrupted"} {
		t.Run(scenario, func(t *testing.T) {
			s, original, device := service(t)
			startFiles(t, s, device)
			ctx := context.Background()
			operation, err := s.AppAction(ctx, device, state.Random(), "files", "stop")
			if err != nil {
				t.Fatal(err)
			}
			s.Backend = &alteredAppReplyBackend{testBackend: original, alter: func(instance supervisor.Instance) supervisor.Instance {
				switch scenario {
				case "id":
					instance.ID = state.Random()
				case "revision":
					instance.Revision++
				case "state":
					instance.State = "running"
				case "workload":
					instance.Workload = "ai"
				case "empty-workload":
					instance.Workload = ""
				case "superseded":
					if _, err := s.Store.DB.Exec("UPDATE apps SET operation_id=?,revision=revision+1,state='starting' WHERE workload='files'", state.Random()); err != nil {
						t.Fatal(err)
					}
				case "operation-interrupted":
					if _, err := s.Store.DB.Exec("UPDATE operations SET state='requires-action' WHERE id=?", operation.ID); err != nil {
						t.Fatal(err)
					}
				}
				return instance
			}}
			err = s.processApp(ctx)
			if scenario == "superseded" || scenario == "operation-interrupted" {
				if !errors.Is(err, ErrConflict) {
					t.Fatal("stale completion accepted", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			result, err := s.Operation(ctx, device, operation.ID)
			if err != nil || result.State == "succeeded" {
				t.Fatal("invalid reply reported success", result, err)
			}
			if scenario == "operation-interrupted" {
				apps, err := s.Apps(ctx)
				if err != nil || len(apps) != 1 || apps[0].State != "stopping" {
					t.Fatal("failed completion did not roll back app state", apps, err)
				}
			}
		})
	}
}
