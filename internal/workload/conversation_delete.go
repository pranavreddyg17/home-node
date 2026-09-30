package workload

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
)

type conversationDeleteIntent struct {
	ConversationID string `json:"conversationId"`
}

// RequestConversationDeletionInTransaction records authority before any guest
// side effect. The operation is complete only after guest deletion acknowledgements
// and management metadata cleanup; insertion alone never means data was deleted.
func (s *Service) RequestConversationDeletionInTransaction(tx *sql.Tx, device, key, id string) (Operation, error) {
	if !guestproto.ValidID(id) {
		return Operation{}, ErrInvalid
	}
	var exists, active int
	if err := tx.QueryRow("SELECT count(*) FROM conversations WHERE id=?", id).Scan(&exists); err != nil {
		return Operation{}, err
	}
	if exists != 1 {
		return Operation{}, ErrInvalid
	}
	if err := tx.QueryRow("SELECT count(*) FROM generations WHERE conversation_id=? AND state IN('staging','queued','running','cancelling')", id).Scan(&active); err != nil {
		return Operation{}, err
	}
	if active > 0 {
		return Operation{}, ErrConflict
	}
	intent := conversationDeleteIntent{ConversationID: id}
	op, replay, err := operation(tx, device, key, "conversation.delete", intent)
	if err != nil || replay {
		return op, err
	}
	var pending bool
	err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM operations WHERE kind='conversation.delete' AND state IN('pending','running') AND id<>? AND json_extract(result,'$.conversationId')=?)", op.ID, id).Scan(&pending)
	if err != nil {
		return Operation{}, err
	}
	if pending {
		return Operation{}, ErrConflict
	}
	data, err := json.Marshal(intent)
	if err != nil {
		return Operation{}, err
	}
	if _, err := tx.Exec("UPDATE operations SET result=? WHERE id=?", string(data), op.ID); err != nil {
		return Operation{}, err
	}
	op.Result = data
	return op, nil
}

func conversationDeletionPending(tx *sql.Tx, id string) (bool, error) {
	var count int
	err := tx.QueryRow("SELECT count(*) FROM operations WHERE kind='conversation.delete' AND state IN('pending','running') AND json_extract(result,'$.conversationId')=?", id).Scan(&count)
	return count > 0, err
}

func (s *Service) processConversationDeletion(ctx context.Context) error {
	var operationID, device, payload string
	err := s.Store.DB.QueryRowContext(ctx, "SELECT id,device_id,result FROM operations WHERE kind='conversation.delete' AND state='pending' ORDER BY created_at LIMIT 1").Scan(&operationID, &device, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var object map[string]json.RawMessage
	var intent conversationDeleteIntent
	if len(payload) > 256 || json.Unmarshal([]byte(payload), &object) != nil || len(object) != 1 || object["conversationId"] == nil || json.Unmarshal([]byte(payload), &intent) != nil || !guestproto.ValidID(intent.ConversationID) {
		return ErrConflict
	}
	deadline, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return s.deleteConversation(deadline, device, intent.ConversationID, operationID)
}
