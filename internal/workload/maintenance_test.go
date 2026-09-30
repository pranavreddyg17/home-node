package workload

import (
	"context"
	"database/sql"
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

func TestMaintenancePreservesFilesAndRejectsNewDeletionIntent(t *testing.T) {
	s, _, device := service(t)
	ctx := context.Background()
	startFiles(t, s, device)
	data := []byte("maintenance fixture")
	transfer, err := s.CreateTransfer(ctx, device, "original.txt", int64(len(data)), sum(data))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Upload(ctx, device, transfer.ID, 0, data, sum(data)); err != nil {
		t.Fatal(err)
	}
	file, err := s.Finalize(ctx, device, transfer.ID)
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := s.CreateConversation(ctx, "retained")
	if err != nil {
		t.Fatal(err)
	}
	token, err := s.Store.BeginMaintenance(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"rename", "trash", "restore"} {
		if err = s.ChangeFile(ctx, device, file.ID, action, "changed.txt"); !errors.Is(err, state.ErrMaintenance) {
			t.Fatal(action, err)
		}
	}
	stored, err := s.File(ctx, file.ID)
	if err != nil || stored.Name != file.Name || stored.TrashUntil != nil {
		t.Fatal("maintenance changed file metadata", stored, err)
	}
	err = s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		_, err := s.RequestConversationDeletionInTransaction(tx, device, state.Random(), conversation.ID)
		return err
	})
	if !errors.Is(err, state.ErrMaintenance) {
		t.Fatal("maintenance admitted deletion", err)
	}
	var count int
	if err = s.Store.DB.QueryRow("SELECT count(*) FROM operations WHERE kind='conversation.delete'").Scan(&count); err != nil || count != 0 {
		t.Fatal("rejected deletion retained authority", count, err)
	}
	if err = s.Store.EndMaintenance(ctx, token); err != nil {
		t.Fatal(err)
	}
	if err = s.ChangeFile(ctx, device, file.ID, "rename", "changed.txt"); err != nil {
		t.Fatal("release did not allow file change", err)
	}
}

func TestMaintenancePausesTrashExpiryUntilRelease(t *testing.T) {
	s, _, device := service(t)
	startFiles(t, s, device)
	ctx := context.Background()
	data := []byte("retained during maintenance")
	transfer, err := s.CreateTransfer(ctx, device, "expired.txt", int64(len(data)), sum(data))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Upload(ctx, device, transfer.ID, 0, data, sum(data)); err != nil {
		t.Fatal(err)
	}
	file, err := s.Finalize(ctx, device, transfer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Store.DB.Exec("UPDATE files SET trash_until=1 WHERE id=?", file.ID); err != nil {
		t.Fatal(err)
	}
	token, err := s.Store.BeginMaintenance(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.expireTrash(ctx); err != nil {
		t.Fatal(err)
	}
	var size int64
	if err = s.Store.DB.QueryRow("SELECT size FROM files WHERE id=?", file.ID).Scan(&size); err != nil || size != int64(len(data)) {
		t.Fatal("maintenance reclaimed bytes", size, err)
	}
	ready, err := s.Store.MaintenanceActivitiesDrained(ctx, token)
	if err != nil || !ready {
		t.Fatal("paused worker leaked activity", ready, err)
	}
	if err = s.Store.EndMaintenance(ctx, token); err != nil {
		t.Fatal(err)
	}
	if err = s.expireTrash(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.Store.DB.QueryRow("SELECT size FROM files WHERE id=?", file.ID).Scan(&size); err != nil || size != 0 {
		t.Fatal("released cleanup did not reclaim", size, err)
	}
	var count int
	if err = s.Store.DB.QueryRow("SELECT count(*) FROM settings WHERE key GLOB 'host.activity.*'").Scan(&count); err != nil || count != 0 {
		t.Fatal("completed worker leaked activity", count, err)
	}
}

func TestMaintenanceInventoryTracksPendingWorkAndUnknownStates(t *testing.T) {
	s, _, device, conversation := aiService(t)
	ctx := context.Background()
	generation, err := s.CreateGeneration(ctx, device, state.Random(), conversation.ID, "inventory fixture")
	if err != nil {
		t.Fatal(err)
	}
	token, err := s.Store.BeginMaintenance(ctx)
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := s.Store.InspectMaintenance(ctx, token)
	if err != nil || inventory.Generations != 1 || inventory.Operations != 1 || inventory.Apps != 1 {
		t.Fatal("pending work missing from inventory", inventory, err)
	}
	if _, err = s.Store.InspectMaintenance(ctx, state.Random()); !errors.Is(err, state.ErrMaintenanceOwner) {
		t.Fatal("foreign token inspected inventory", err)
	}
	if _, err = s.Store.DB.Exec("UPDATE generations SET state='unrecognized' WHERE id=?", generation.ID); err != nil {
		t.Fatal(err)
	}
	inventory, err = s.Store.InspectMaintenance(ctx, token)
	if err != nil || inventory.Generations != 1 {
		t.Fatal("unknown state treated as drained", inventory, err)
	}
	if _, err = s.Store.DB.Exec("INSERT INTO settings VALUES('job.cleanup.fixture','malformed-json')"); err != nil {
		t.Fatal(err)
	}
	inventory, err = s.Store.InspectMaintenance(ctx, token)
	if err != nil || inventory.Cleanup != 1 {
		t.Fatal("invalid cleanup record treated as drained", inventory, err)
	}
	if _, err = s.Store.DB.Exec("UPDATE generations SET state='failed' WHERE id=?", generation.ID); err != nil {
		t.Fatal(err)
	}
	inventory, err = s.Store.InspectMaintenance(ctx, token)
	if err != nil || inventory.Generations != 0 || inventory.Operations != 1 {
		t.Fatal("inventory conflated generation and operation completion", inventory, err)
	}
}
