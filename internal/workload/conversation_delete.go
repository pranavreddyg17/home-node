package workload

import (
	"database/sql"
	"encoding/json"

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
