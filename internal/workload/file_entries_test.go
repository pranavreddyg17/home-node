package workload

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestFileEntryLimitReservesConcurrentUploadsAndJobOutputs(t *testing.T) {
	s, _, device := service(t)
	ctx := context.Background()
	startFiles(t, s, device)
	input, err := s.CreateTransfer(ctx, device, "empty.mp4", 0, sum(nil))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Finalize(ctx, device, input.ID); err != nil {
		t.Fatal(err)
	}
	err = s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		statement, err := tx.Prepare("INSERT INTO files(id,name,size,sha256,created_at,trash_until) VALUES(?,'fixture',0,?,1,NULL)")
		if err != nil {
			return err
		}
		defer statement.Close()
		for i := 0; i < FileEntryLimit-2; i++ {
			if _, err = statement.Exec(state.Random(), sum(nil)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		transfer Transfer
		key      string
		err      error
	}
	results := make(chan result, 8)
	for i := 0; i < 8; i++ {
		go func() {
			key := state.Random()
			transfer, err := s.CreateTransferWithKey(ctx, device, "new empty", 0, sum(nil), key)
			results <- result{transfer, key, err}
		}()
	}
	var winner result
	accepted := 0
	for i := 0; i < 8; i++ {
		r := <-results
		if r.err == nil {
			accepted++
			winner = r
		} else if !errors.Is(r.err, ErrCapacity) {
			t.Fatal(r.err)
		}
	}
	if accepted != 1 {
		t.Fatal("concurrent creation exceeded reserved entry capacity", accepted)
	}
	usage, err := s.FilesStorage(ctx)
	if err != nil || usage.StoredFileEntries != FileEntryLimit-1 || usage.ReservedFileEntries != 1 || usage.AvailableFileEntries != 0 {
		t.Fatal(usage, err)
	}
	replay, err := s.CreateTransferWithKey(ctx, device, "new empty", 0, sum(nil), winner.key)
	if err != nil || replay.ID != winner.transfer.ID {
		t.Fatal("full capacity hid creation replay", replay, err)
	}
	if _, err = s.Finalize(ctx, device, winner.transfer.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.ChangeFile(ctx, device, winner.transfer.ID, "trash", ""); err != nil {
		t.Fatal(err)
	}
	usage, err = s.FilesStorage(ctx)
	if err != nil || usage.StoredFileEntries != FileEntryLimit || usage.AvailableFileEntries != 0 {
		t.Fatal("Trash released entry before cleanup", usage, err)
	}
	if _, err = s.Store.DB.Exec("UPDATE files SET trash_until=0 WHERE id=?", winner.transfer.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.expireTrash(ctx); err != nil {
		t.Fatal(err)
	}
	job, err := s.CreateJob(ctx, device, state.Random(), input.ID, "mp4-720p", "")
	if err != nil {
		t.Fatal(err)
	}
	usage, err = s.FilesStorage(ctx)
	if err != nil || usage.ReservedFileEntries != 1 || usage.AvailableFileEntries != 0 {
		t.Fatal("job output did not reserve an entry", usage, err)
	}
	if _, err = s.CreateTransfer(ctx, device, "blocked", 0, sum(nil)); !errors.Is(err, ErrCapacity) {
		t.Fatal("upload ignored job entry reservation", err)
	}
	if err = s.CancelJob(ctx, device, job.ID, job.AttemptID); err != nil {
		t.Fatal(err)
	}
	usage, err = s.FilesStorage(ctx)
	if err != nil || usage.ReservedFileEntries != 0 || usage.AvailableFileEntries != 1 {
		t.Fatal("never-started cancellation retained entry", usage, err)
	}
}
