package workload

import (
	"context"
	"errors"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestConversationCreationReplaysDuringMaintenanceAndNeverRecreatesDeletedChat(t *testing.T) {
	s, _, device := service(t)
	ctx := context.Background()
	key := state.Random()
	first, err := s.CreateConversationWithKey(ctx, device, key, "Retry fixture")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Store.BeginMaintenance(ctx); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		replay, err := s.CreateConversationWithKey(ctx, device, key, "Retry fixture")
		if err != nil || replay.ID != first.ID {
			t.Fatal("creation replay changed identity", replay, err)
		}
	}
	if _, err = s.CreateConversationWithKey(ctx, device, key, "Different fixture"); !errors.Is(err, ErrConflict) {
		t.Fatal("changed intent reused key", err)
	}
	if _, err = s.CreateConversationWithKey(ctx, device, state.Random(), "New fixture"); !errors.Is(err, state.ErrMaintenance) {
		t.Fatal("new creation bypassed maintenance", err)
	}
	if _, err = s.Store.DB.Exec("DELETE FROM conversations WHERE id=?", first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateConversationWithKey(ctx, device, key, "Retry fixture"); err == nil {
		t.Fatal("deleted conversation was recreated")
	}
	var count int
	if err = s.Store.DB.QueryRow("SELECT count(*) FROM conversations").Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
}

func TestConcurrentConversationCreationUsesOneDurableIdentity(t *testing.T) {
	s, _, device := service(t)
	key := state.Random()
	type result struct {
		item Conversation
		err  error
	}
	results := make(chan result, 8)
	for i := 0; i < 8; i++ {
		go func() {
			item, err := s.CreateConversationWithKey(context.Background(), device, key, "Concurrent fixture")
			results <- result{item, err}
		}()
	}
	id := ""
	for i := 0; i < 8; i++ {
		r := <-results
		if r.err != nil {
			t.Fatal(r.err)
		}
		if id == "" {
			id = r.item.ID
		}
		if r.item.ID != id {
			t.Fatal("concurrent replay duplicated conversation", id, r.item)
		}
	}
	var count int
	if err := s.Store.DB.QueryRow("SELECT count(*) FROM conversations").Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
}
