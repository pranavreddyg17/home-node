package workload

import (
	"context"
	"errors"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestNeverStartedJobCancelsAtomicallyWithoutRuntime(t *testing.T) {
	for _, phase := range []string{"queued", "preparing", "cancelling"} {
		t.Run(phase, func(t *testing.T) {
			s, backend, device, input := jobService(t)
			ctx := context.Background()
			job, err := s.CreateJob(ctx, device, state.Random(), input.ID, "mp4-720p", "")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.Store.DB.Exec("UPDATE jobs SET state=? WHERE id=?", phase, job.AttemptID); err != nil {
				t.Fatal(err)
			}
			s.Backend = nil
			usage, err := s.FilesStorage(ctx)
			if err != nil || usage.JobReservedBytes != MaxJobOutputBytes {
				t.Fatal("missing pre-cancellation reservation", usage, err)
			}
			if _, err = s.Store.DB.Exec(`CREATE TRIGGER fixture_job_cancel BEFORE UPDATE ON operations WHEN NEW.state='failed' BEGIN SELECT RAISE(ABORT,'fixture cancellation write failure'); END`); err != nil {
				t.Fatal(err)
			}
			if err = s.CancelJob(ctx, device, job.ID, job.AttemptID); err == nil {
				t.Fatal("cancellation failure hidden")
			}
			current, err := s.attempt(ctx, job.AttemptID)
			if err != nil || current.State != phase {
				t.Fatal("partial cancellation committed", current, err)
			}
			usage, err = s.FilesStorage(ctx)
			if err != nil || usage.JobReservedBytes != MaxJobOutputBytes {
				t.Fatal("failed cancellation released quota", usage, err)
			}
			if _, err = s.Store.DB.Exec("DROP TRIGGER fixture_job_cancel"); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				if err = s.CancelJob(ctx, device, job.ID, job.AttemptID); err != nil {
					t.Fatal(err)
				}
			}
			current, err = s.attempt(ctx, job.AttemptID)
			if err != nil || current.State != "cancelled" || current.StartRequested {
				t.Fatal(current, err)
			}
			usage, err = s.FilesStorage(ctx)
			if err != nil || usage.JobReservedBytes != 0 {
				t.Fatal("confirmed never-started cancellation retained quota", usage, err)
			}
			s.Backend = backend
			if err = s.executeJob(ctx, job); !errors.Is(err, context.Canceled) {
				t.Fatal("stale worker did not refuse cancelled admission", err)
			}
			op, err := s.Operation(ctx, device, job.OperationID)
			if err != nil || op.State != "failed" {
				t.Fatal(op, err)
			}
			var events int
			if err = s.Store.DB.QueryRow("SELECT count(*) FROM events WHERE kind='job.cancelled' AND object_id=?", job.ID).Scan(&events); err != nil || events != 1 {
				t.Fatal(events, err)
			}
			if backend.runs != 0 || backend.stops != 0 || backend.purges != 0 || backend.videoID != "" {
				t.Fatal("never-started cancellation dispatched runtime effects")
			}
		})
	}
}

func TestJobCancellationRetainsUncertainRuntimeAuthority(t *testing.T) {
	for _, started := range []bool{false, true} {
		s, backend, device, input := jobService(t)
		ctx := context.Background()
		job, err := s.CreateJob(ctx, device, state.Random(), input.ID, "mp4-720p", "")
		if err != nil {
			t.Fatal(err)
		}
		if started {
			if err = s.jobPhase(ctx, job, "preparing"); err != nil {
				t.Fatal(err)
			}
			if _, err = s.Store.DB.Exec("UPDATE jobs SET start_requested=1 WHERE id=?", job.AttemptID); err != nil {
				t.Fatal(err)
			}
		}
		if _, err = s.Store.DB.Exec("INSERT INTO settings(key,value) VALUES(?,?)", cleanupKey(job.AttemptID), `{"state":"pending","lastAttempt":0}`); err != nil {
			t.Fatal(err)
		}
		s.Backend = nil
		err = s.CancelJob(ctx, device, job.ID, job.AttemptID)
		if started && err != nil || !started && !errors.Is(err, ErrConflict) {
			t.Fatal("uncertain cleanup admission", started, err)
		}
		current, err := s.attempt(ctx, job.AttemptID)
		if err != nil || current.State == "cancelled" {
			t.Fatal("unconfirmed runtime treated as stopped", current, err)
		}
		usage, err := s.FilesStorage(ctx)
		if err != nil || usage.JobReservedBytes != MaxJobOutputBytes {
			t.Fatal("uncertain runtime released reservation", usage, err)
		}
		if backend.runs != 0 || backend.stops != 0 {
			t.Fatal("unavailable runtime dispatched effects")
		}
	}
}
