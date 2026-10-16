package workload

import (
	"context"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestLateJobCompletionPreservesEveryTerminalOutcome(t *testing.T) {
	for _, phase := range []string{"succeeded", "failed", "cancelled", "interrupted"} {
		t.Run(phase, func(t *testing.T) {
			s, backend, device, input := jobService(t)
			ctx := context.Background()
			job, err := s.CreateJob(ctx, device, state.Random(), input.ID, "mp4-720p", "")
			if err != nil {
				t.Fatal(err)
			}
			if phase == "succeeded" {
				err = s.processJob(ctx)
			} else {
				err = s.endJob(ctx, job, phase, "INITIAL_FIXTURE", "")
			}
			if err != nil {
				t.Fatal(err)
			}
			original, err := s.Operation(ctx, device, job.OperationID)
			if err != nil {
				t.Fatal(err)
			}
			var before int
			if err = s.Store.DB.QueryRow("SELECT count(*) FROM events WHERE object_id=? AND kind GLOB 'job.*'", job.ID).Scan(&before); err != nil {
				t.Fatal(err)
			}
			for _, late := range []string{"failed", "cancelled", "interrupted"} {
				if err = s.endJob(ctx, job, late, "LATE_FIXTURE", ""); err != nil {
					t.Fatal(err)
				}
			}
			latest, err := s.attempt(ctx, job.AttemptID)
			if err != nil || latest.State != phase {
				t.Fatal("terminal attempt overwritten", latest, err)
			}
			op, err := s.Operation(ctx, device, job.OperationID)
			if err != nil || op.State != original.State || string(op.Result) != string(original.Result) {
				t.Fatal("terminal operation overwritten", op, original, err)
			}
			var after int
			if err = s.Store.DB.QueryRow("SELECT count(*) FROM events WHERE object_id=? AND kind GLOB 'job.*'", job.ID).Scan(&after); err != nil || before != after {
				t.Fatal("late completion duplicated events", before, after, err)
			}
			if phase != "succeeded" && (backend.runs != 0 || backend.stops != 0) {
				t.Fatal("metadata completion dispatched runtime effects")
			}
		})
	}
}
