package workload

import (
	"context"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestOrphanCleanupRetainsReservationUntilGuestDeletion(t *testing.T) {
	s, backend, device, _ := jobService(t)
	ctx := context.Background()
	instance, err := s.app(ctx, "files")
	if err != nil {
		t.Fatal(err)
	}
	id := state.Random()
	data := []byte("partial copied result")
	if _, err = s.call(ctx, instance, guestproto.Request{Operation: "upload", ObjectID: id, Size: int64(len(data)), Data: data, SHA256: sum(data)}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Store.DB.Exec("INSERT INTO orphan_objects VALUES(?,'files',?)", id, time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	// Three unknown outputs retain the whole storage budget after failed jobs.
	for range 2 {
		if _, err = s.Store.DB.Exec("INSERT INTO orphan_objects VALUES(?,'files',?)", state.Random(), time.Now().Unix()+1); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = s.CreateTransfer(ctx, device, "new", 1, sum([]byte("x"))); err != ErrConflict {
		t.Fatal("orphan quota released early", err)
	}
	s.Backend = nil
	if err = s.cleanupOrphanObject(ctx); err == nil {
		t.Fatal("offline deletion acknowledged")
	}
	var count int
	if err = s.Store.DB.QueryRow("SELECT count(*) FROM orphan_objects").Scan(&count); err != nil || count != 3 {
		t.Fatal(count, err)
	}
	restarted := New(s.Store, backend, s.PolicyGeneration)
	if err = restarted.cleanupOrphanObject(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = restarted.call(ctx, instance, guestproto.Request{Operation: "finalize", ObjectID: id, Size: int64(len(data)), SHA256: sum(data)}); err == nil {
		t.Fatal("deleted partial object survived")
	}
	if _, err = restarted.CreateTransfer(ctx, device, "new", 1, sum([]byte("x"))); err != nil {
		t.Fatal("reservation not released", err)
	}
}

func TestOrphanCleanupRefusesPublishedFile(t *testing.T) {
	s, _, _, input := jobService(t)
	if _, err := s.Store.DB.Exec("INSERT INTO orphan_objects VALUES(?,'files',0)", input.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.cleanupOrphanObject(context.Background()); err != ErrConflict {
		t.Fatal(err)
	}
	if _, err := s.File(context.Background(), input.ID); err != nil {
		t.Fatal(err)
	}
}
