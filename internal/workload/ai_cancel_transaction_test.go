package workload

import (
	"context"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestUnstartedGenerationCancellationCommitsTogether(t *testing.T) {
	for _, phase := range []string{"staging", "queued"} {
		t.Run(phase, func(t *testing.T) {
			s, backend, device, conversation := aiService(t)
			ctx := context.Background()
			if phase == "staging" {
				s.Backend = &interruptedAIUpload{aiBackend: backend, failUpload: true}
			}
			g, err := s.CreateGeneration(ctx, device, state.Random(), conversation.ID, "Cancellation transaction fixture")
			if phase == "queued" && err != nil || phase == "staging" && err == nil {
				t.Fatal("fixture admission", g, err)
			}
			if err = s.Store.DB.QueryRow("SELECT id,operation_id FROM generations WHERE conversation_id=?", conversation.ID).Scan(&g.ID, &g.OperationID); err != nil {
				t.Fatal(err)
			}
			if _, err = s.Store.DB.Exec(`CREATE TRIGGER fixture_cancel BEFORE UPDATE ON operations WHEN NEW.state='failed' BEGIN SELECT RAISE(ABORT,'fixture cancellation write failure'); END`); err != nil {
				t.Fatal(err)
			}
			if err = s.CancelGeneration(ctx, g.ID); err == nil {
				t.Fatal("failed operation write did not roll back cancellation")
			}
			var generationState, operationState string
			if err = s.Store.DB.QueryRow("SELECT g.state,o.state FROM generations g JOIN operations o ON o.id=g.operation_id WHERE g.id=?", g.ID).Scan(&generationState, &operationState); err != nil || generationState != phase || operationState != "pending" {
				t.Fatal("partial cancellation committed", generationState, operationState, err)
			}
			if _, err = s.Store.DB.Exec("DROP TRIGGER fixture_cancel"); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				if err = s.CancelGeneration(ctx, g.ID); err != nil {
					t.Fatal(err)
				}
			}
			for _, late := range []string{"failed", "succeeded"} {
				if err = s.endGeneration(ctx, g, late, "LATE_FIXTURE_COMPLETION"); err != nil {
					t.Fatal(err)
				}
			}
			if err = s.Store.DB.QueryRow("SELECT g.state,o.state FROM generations g JOIN operations o ON o.id=g.operation_id WHERE g.id=?", g.ID).Scan(&generationState, &operationState); err != nil || generationState != "cancelled" || operationState != "failed" {
				t.Fatal(generationState, operationState, err)
			}
			var events int
			if err = s.Store.DB.QueryRow("SELECT count(*) FROM events WHERE kind='generation.cancelled' AND object_id=?", g.ID).Scan(&events); err != nil || events != 1 || backend.calls != 0 {
				t.Fatal("cancellation duplicated event or runtime effect", events, backend.calls, err)
			}
		})
	}
}
