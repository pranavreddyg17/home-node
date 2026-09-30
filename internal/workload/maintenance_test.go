package workload

import (
	"context"
	"errors"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestMaintenanceBlocksWorkloadAdmissionWithoutNewIntents(t *testing.T) {
	s, _, device, conversation := aiService(t)
	ctx := context.Background()
	if _, err := s.Store.DB.Exec("INSERT INTO apps(workload,instance_id,state,updated_at,revision) VALUES('files',?,'running',1,1)", state.Random()); err != nil {
		t.Fatal(err)
	}
	var before int
	var err error
	if err = s.Store.DB.QueryRow("SELECT (SELECT count(*) FROM operations)+(SELECT count(*) FROM transfers)+(SELECT count(*) FROM generations)+(SELECT count(*) FROM jobs)").Scan(&before); err != nil {
		t.Fatal(err)
	}
	token, err := s.Store.BeginMaintenance(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var existingID, existingKey string
	if err = s.Store.DB.QueryRow("SELECT id,idempotency_key FROM operations WHERE kind='app.start' AND device_id=?", device).Scan(&existingID, &existingKey); err != nil {
		t.Fatal(err)
	}
	replay, err := s.AppAction(ctx, device, existingKey, "ai", "start")
	if err != nil || replay.ID != existingID {
		t.Fatal("maintenance blocked observation of admitted intent", replay, err)
	}
	for name, call := range map[string]func() error{
		"upload": func() error { _, err := s.CreateTransfer(ctx, device, "fixture", 0, sum(nil)); return err },
		"job": func() error {
			_, err := s.CreateJob(ctx, device, state.Random(), state.Random(), "mp4-720p", "")
			return err
		},
		"generation": func() error {
			_, err := s.CreateGeneration(ctx, device, state.Random(), conversation.ID, "fixture")
			return err
		},
		"conversation": func() error { _, err := s.CreateConversation(ctx, "fixture"); return err },
		"app":          func() error { _, err := s.AppAction(ctx, device, state.Random(), "ai", "stop"); return err },
	} {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, state.ErrMaintenance) {
				t.Fatal("maintenance admission accepted", err)
			}
		})
	}
	var count int
	if err = s.Store.DB.QueryRow("SELECT (SELECT count(*) FROM operations)+(SELECT count(*) FROM transfers)+(SELECT count(*) FROM generations)+(SELECT count(*) FROM jobs)").Scan(&count); err != nil || count != before {
		t.Fatal("rejection persisted work", count, err)
	}
	if err = s.Store.EndMaintenance(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateConversation(ctx, "after maintenance"); err != nil {
		t.Fatal("release failed to reopen admission", err)
	}
}
