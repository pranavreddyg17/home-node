package workload

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestConversationDeletionIntentIsTransactionalAndNotCompletion(t *testing.T) {
	s, _, device := service(t)
	conversation, err := s.CreateConversation(context.Background(), device)
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("injected failure")
	key := state.Random()
	err = s.Store.Transaction(context.Background(), func(tx *sql.Tx) error {
		if _, err := s.RequestConversationDeletionInTransaction(tx, device, key, conversation.ID); err != nil {
			return err
		}
		return failure
	})
	if !errors.Is(err, failure) {
		t.Fatal(err)
	}
	var count int
	if err := s.Store.DB.QueryRow("SELECT count(*) FROM operations WHERE kind='conversation.delete'").Scan(&count); err != nil || count != 0 {
		t.Fatal("rollback retained intent", count, err)
	}
	var first Operation
	for i := 0; i < 2; i++ {
		err = s.Store.Transaction(context.Background(), func(tx *sql.Tx) error {
			op, err := s.RequestConversationDeletionInTransaction(tx, device, key, conversation.ID)
			if err != nil {
				return err
			}
			if i == 0 {
				first = op
			} else if op.ID != first.ID {
				t.Fatal("retry duplicated deletion")
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if first.State != "pending" {
		t.Fatal("intent misreported completion", first)
	}
	if err := s.Store.DB.QueryRow("SELECT count(*) FROM conversations WHERE id=?", conversation.ID).Scan(&count); err != nil || count != 1 {
		t.Fatal("intent prematurely deleted conversation", count, err)
	}
}
