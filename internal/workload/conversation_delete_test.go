package workload

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

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
	listed, err := s.Conversations(context.Background())
	if err != nil || len(listed) != 1 || !listed[0].DeletionPending {
		t.Fatal("pending deletion absent from reload state", listed, err)
	}
	if first.State != "pending" {
		t.Fatal("intent misreported completion", first)
	}
	if err := s.Store.DB.QueryRow("SELECT count(*) FROM conversations WHERE id=?", conversation.ID).Scan(&count); err != nil || count != 1 {
		t.Fatal("intent prematurely deleted conversation", count, err)
	}
}

func TestPendingConversationDeletionBlocksNewIntentAndAdmission(t *testing.T) {
	s, _, device := service(t)
	conversation, err := s.CreateConversation(context.Background(), device)
	if err != nil {
		t.Fatal(err)
	}
	key := state.Random()
	if err := s.Store.Transaction(context.Background(), func(tx *sql.Tx) error {
		_, err := s.RequestConversationDeletionInTransaction(tx, device, key, conversation.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.Transaction(context.Background(), func(tx *sql.Tx) error {
		pending, err := conversationDeletionPending(tx, conversation.ID)
		if err != nil {
			return err
		}
		if !pending {
			t.Fatal("deletion admission not closed")
		}
		other, err := conversationDeletionPending(tx, state.Random())
		if err != nil {
			return err
		}
		if other {
			t.Fatal("unrelated conversation blocked")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.Transaction(context.Background(), func(tx *sql.Tx) error {
		_, err := s.RequestConversationDeletionInTransaction(tx, device, state.Random(), conversation.ID)
		return err
	}); !errors.Is(err, ErrConflict) {
		t.Fatal("duplicate deletion intent admitted", err)
	}
	if _, err := s.Store.DB.Exec("INSERT INTO apps(workload,instance_id,state,updated_at) VALUES('ai',?,'running',0)", state.Random()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateGeneration(context.Background(), device, state.Random(), conversation.ID, "hello"); !errors.Is(err, ErrConflict) {
		t.Fatal("generation entered deleting conversation", err)
	}
	var generations int
	if err := s.Store.DB.QueryRow("SELECT count(*) FROM generations").Scan(&generations); err != nil || generations != 0 {
		t.Fatal("blocked generation persisted", generations, err)
	}
	var count int
	if err := s.Store.DB.QueryRow("SELECT count(*) FROM operations WHERE kind='conversation.delete'").Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate intent retained", count, err)
	}
}

func TestDeletionWorkerCompletesMetadataAndOperationAtomically(t *testing.T) {
	s, _, device := service(t)
	ctx := context.Background()
	conversation, err := s.CreateConversation(ctx, device)
	if err != nil {
		t.Fatal(err)
	}
	var op Operation
	if err := s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		var err error
		op, err = s.RequestConversationDeletionInTransaction(tx, device, state.Random(), conversation.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.processConversationDeletion(ctx); err != nil {
		t.Fatal(err)
	}
	completed, err := s.Operation(ctx, device, op.ID)
	if err != nil || completed.State != "succeeded" || string(completed.Result) != `{"deleted":true}` {
		t.Fatal("completion not committed", completed, err)
	}
	var count int
	if err := s.Store.DB.QueryRow("SELECT count(*) FROM conversations WHERE id=?", conversation.ID).Scan(&count); err != nil || count != 0 {
		t.Fatal("conversation retained", count, err)
	}
	if err := s.processConversationDeletion(ctx); err != nil {
		t.Fatal("idle replay failed", err)
	}
}

func TestDeletionWorkerRetriesAfterUnavailableGuestAndServiceRestart(t *testing.T) {
	s, backend, device, conversation := aiService(t)
	ctx := context.Background()
	if _, err := s.CreateGeneration(ctx, device, state.Random(), conversation.ID, "private fixture"); err != nil {
		t.Fatal(err)
	}
	if err := s.processGeneration(ctx); err != nil {
		t.Fatal(err)
	}
	var op Operation
	if err := s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		var err error
		op, err = s.RequestConversationDeletionInTransaction(tx, device, state.Random(), conversation.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Store.DB.Exec("UPDATE apps SET state='stopped' WHERE workload='ai'"); err != nil {
		t.Fatal(err)
	}
	if err := s.processConversationDeletion(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatal("unavailable guest acknowledged deletion", err)
	}
	pending, err := s.Operation(ctx, device, op.ID)
	if err != nil || pending.State != "pending" {
		t.Fatal("failed deletion lost intent", pending, err)
	}
	var count int
	if err := s.Store.DB.QueryRow("SELECT count(*) FROM generations WHERE conversation_id=?", conversation.ID).Scan(&count); err != nil || count != 1 {
		t.Fatal("failure removed metadata", count, err)
	}
	var retryPayload string
	if err := s.Store.DB.QueryRow("SELECT result FROM operations WHERE id=?", op.ID).Scan(&retryPayload); err != nil {
		t.Fatal(err)
	}
	var retry conversationDeleteIntent
	if err := json.Unmarshal([]byte(retryPayload), &retry); err != nil || retry.Attempts != 1 || retry.NextAttemptAt <= time.Now().Unix() {
		t.Fatal("retry schedule not persisted", retry, err)
	}
	second, err := s.CreateConversation(ctx, "another deletion")
	if err != nil {
		t.Fatal(err)
	}
	var secondOp Operation
	if err := s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		var err error
		secondOp, err = s.RequestConversationDeletionInTransaction(tx, device, state.Random(), second.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.processConversationDeletion(ctx); err != nil {
		t.Fatal("deferred failure starved eligible deletion", err)
	}
	secondStatus, err := s.Operation(ctx, device, secondOp.ID)
	if err != nil || secondStatus.State != "succeeded" {
		t.Fatal("eligible deletion did not complete", secondStatus, err)
	}
	// Simulate reaching the persisted eligibility time without a wall-clock sleep.
	if _, err := s.Store.DB.Exec("UPDATE operations SET result=json_set(result,'$.nextAttemptAt',0) WHERE id=?", op.ID); err != nil {
		t.Fatal(err)
	}
	restarted := New(s.Store, backend, 1)
	if _, err := s.Store.DB.Exec("UPDATE apps SET state='running' WHERE workload='ai'"); err != nil {
		t.Fatal(err)
	}
	if err := restarted.processConversationDeletion(ctx); err != nil {
		t.Fatal("restart retry failed", err)
	}
	completed, err := restarted.Operation(ctx, device, op.ID)
	if err != nil || completed.State != "succeeded" {
		t.Fatal("retry did not complete", completed, err)
	}
}
