package workload

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestMaintenanceStopOwnsBarrierAndPreservesAdmission(t *testing.T) {
	s, original, device := service(t)
	startFiles(t, s, device)
	b := &appShutdownBackend{testBackend: original}
	s.Backend = b
	ctx := context.Background()
	token, err := s.Store.BeginMaintenance(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.MaintenanceStop(ctx, state.Random(), device, "files"); !errors.Is(err, state.ErrMaintenanceOwner) {
		t.Fatal(err)
	}
	op, err := s.MaintenanceStop(ctx, token, device, "files")
	if err != nil {
		t.Fatal(err)
	}
	var intent appIntent
	if err = json.Unmarshal(op.Result, &intent); err != nil || !intent.CooperativeStop || len(b.requests) != 0 {
		t.Fatal(intent, err, b.requests)
	}
	replay, err := s.MaintenanceStop(ctx, token, device, "files")
	if err != nil || replay.ID != op.ID {
		t.Fatal(replay, err)
	}
	if _, err = s.AppAction(ctx, device, state.Random(), "ai", "start"); !errors.Is(err, state.ErrMaintenance) {
		t.Fatal("admission reopened", err)
	}
	if err = s.processApp(ctx); err != nil {
		t.Fatal(err)
	}
	if len(b.requests) != 1 || b.requests[0].Action != "shutdown" {
		t.Fatal(b.requests)
	}
	replay, err = s.MaintenanceStop(ctx, token, device, "files")
	if err != nil || replay.ID != op.ID || replay.State != "succeeded" {
		t.Fatal(replay, err)
	}
	if err = s.Store.EndMaintenance(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, err = s.MaintenanceStop(ctx, token, device, "files"); !errors.Is(err, state.ErrMaintenanceOwner) {
		t.Fatal("released token reused", err)
	}
}

func TestMaintenanceStopRefusesUndrainedWorkAndInvalidAuthority(t *testing.T) {
	for _, scenario := range []string{"transfer", "activity", "cleanup", "revoked", "unknown-phase"} {
		t.Run(scenario, func(t *testing.T) {
			s, original, device := service(t)
			startFiles(t, s, device)
			ctx := context.Background()
			if scenario == "transfer" {
				if _, err := s.CreateTransfer(ctx, device, "pending", 1, sum([]byte("x"))); err != nil {
					t.Fatal(err)
				}
			}
			token, err := s.Store.BeginMaintenance(ctx)
			if err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "activity":
				_, err = s.Store.DB.Exec("INSERT INTO settings VALUES('host.activity.fixture','trash-expiry')")
			case "cleanup":
				_, err = s.Store.DB.Exec("INSERT INTO settings VALUES('job.cleanup.fixture','invalid')")
			case "revoked":
				_, err = s.Store.DB.Exec("UPDATE devices SET revoked_at=1 WHERE id=?", device)
			case "unknown-phase":
				_, err = s.Store.DB.Exec("UPDATE apps SET state='unrecognized' WHERE workload='files'")
			}
			if err != nil {
				t.Fatal(err)
			}
			b := &appShutdownBackend{testBackend: original}
			s.Backend = b
			var before, after int
			if err = s.Store.DB.QueryRow("SELECT count(*) FROM operations").Scan(&before); err != nil {
				t.Fatal(err)
			}
			if _, err = s.MaintenanceStop(ctx, token, device, "files"); err == nil {
				t.Fatal("unsafe stop admitted")
			}
			if err = s.Store.DB.QueryRow("SELECT count(*) FROM operations").Scan(&after); err != nil || before != after || len(b.requests) != 0 {
				t.Fatal("rejected action retained effects", before, after, b.requests, err)
			}
		})
	}
}

func TestMaintenanceShutdownFailureRetainsBarrierWithoutForcedFallback(t *testing.T) {
	s, original, device := service(t)
	startFiles(t, s, device)
	ctx := context.Background()
	token, err := s.Store.BeginMaintenance(ctx)
	if err != nil {
		t.Fatal(err)
	}
	b := &appShutdownBackend{testBackend: original, refuse: true}
	s.Backend = b
	op, err := s.MaintenanceStop(ctx, token, device, "files")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.processApp(ctx); err != nil {
		t.Fatal(err)
	}
	result, err := s.Operation(ctx, device, op.ID)
	if err != nil || result.State != "failed" || len(b.requests) != 1 || b.requests[0].Action != "shutdown" {
		t.Fatal(result, b.requests, err)
	}
	inventory, err := s.Store.InspectMaintenance(ctx, token)
	if err != nil || inventory.Apps != 1 {
		t.Fatal("failure treated as drained", inventory, err)
	}
	if _, err = s.CreateConversation(ctx, "blocked after failure"); !errors.Is(err, state.ErrMaintenance) {
		t.Fatal("failure reopened admission", err)
	}
}
