package workload

import (
	"context"
	"errors"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

type purgeFailureBackend struct {
	*jobBackend
	fail     bool
	requests []supervisor.Request
}

func (b *purgeFailureBackend) Apply(ctx context.Context, r supervisor.Request) (supervisor.Instance, error) {
	b.requests = append(b.requests, r)
	if r.Action == "purge" && b.fail {
		return supervisor.Instance{}, errors.New("temporary volume removal failed")
	}
	return b.jobBackend.Apply(ctx, r)
}
func TestFailedVideoPurgeIsDurableAndRetried(t *testing.T) {
	s, original, device, input := jobService(t)
	backend := &purgeFailureBackend{jobBackend: original, fail: true}
	s.Backend = backend
	ctx := context.Background()
	job, err := s.CreateJob(ctx, device, state.Random(), input.ID, "mp4-720p", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.processJob(ctx); err == nil {
		t.Fatal("purge failure hidden")
	}
	completed, err := s.attempt(ctx, job.AttemptID)
	if err != nil || completed.State != "succeeded" || !completed.CleanupPending || completed.OutputID == nil {
		t.Fatal(completed, err)
	}
	if _, err = s.File(ctx, *completed.OutputID); err != nil {
		t.Fatal("valid result lost", err)
	}
	var status string
	if err = s.Store.DB.QueryRow("SELECT json_extract(value,'$.state') FROM settings WHERE key=?", cleanupKey(job.AttemptID)).Scan(&status); err != nil || status != "pending" {
		t.Fatal("cleanup intent missing", status, err)
	}
	// Recreate the controller service over persistent metadata and retry after the
	// protected runtime becomes available. This does not prove a real VM restart.
	backend.fail = false
	restarted := New(s.Store, backend, s.PolicyGeneration)
	if _, err = s.Store.DB.Exec("UPDATE settings SET value=json_set(value,'$.lastAttempt',0) WHERE key=?", cleanupKey(job.AttemptID)); err != nil {
		t.Fatal(err)
	}
	if err = restarted.cleanupJobResources(ctx); err != nil {
		t.Fatal(err)
	}
	completed, err = restarted.attempt(ctx, job.AttemptID)
	if err != nil || completed.CleanupPending || completed.ErrorCode != nil || completed.State != "succeeded" {
		t.Fatal(completed, err)
	}
	var stopIDs []string
	for _, request := range backend.requests {
		if request.Action == "stop" {
			stopIDs = append(stopIDs, request.OperationID)
		}
	}
	if len(stopIDs) != 2 || stopIDs[0] != stopIDs[1] {
		t.Fatal("retry duplicated supervisor intent", stopIDs)
	}
	before := len(backend.requests)
	if err = restarted.cleanupJobResources(ctx); err != nil || len(backend.requests) != before {
		t.Fatal("completed cleanup repeated", err)
	}
}
