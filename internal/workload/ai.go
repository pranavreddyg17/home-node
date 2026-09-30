package workload

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"github.com/pranavreddyg17/home-node/internal/state"
)

type Conversation struct {
	DeletionRequiresAction bool   `json:"deletionRequiresAction"`
	DeletionPending        bool   `json:"deletionPending"`
	ID                     string `json:"id"`
	Title                  string `json:"title"`
	CreatedAt              int64  `json:"createdAt"`
}
type Generation struct {
	ID             string `json:"id"`
	ConversationID string `json:"conversationId"`
	Prompt         string `json:"prompt"`
	Output         string `json:"output"`
	State          string `json:"state"`
	OperationID    string `json:"operationId"`
	CreatedAt      int64  `json:"createdAt"`
	UpdatedAt      int64  `json:"updatedAt"`
	reference      objectReference
}
type objectReference struct {
	ID     string `json:"id"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

func (s *Service) Conversations(ctx context.Context) ([]Conversation, error) {
	rows, err := s.Store.DB.QueryContext(ctx, "SELECT c.id,c.title,c.created_at,EXISTS(SELECT 1 FROM operations o WHERE o.kind='conversation.delete' AND o.state IN('pending','running','requires-action') AND json_extract(CASE WHEN json_valid(o.result) THEN o.result ELSE '{}' END,'$.conversationId')=c.id),EXISTS(SELECT 1 FROM operations o WHERE o.kind='conversation.delete' AND o.state='requires-action' AND json_extract(CASE WHEN json_valid(o.result) THEN o.result ELSE '{}' END,'$.conversationId')=c.id) FROM conversations c ORDER BY c.created_at DESC LIMIT 100")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Conversation{}
	for rows.Next() {
		var item Conversation
		if err = rows.Scan(&item.ID, &item.Title, &item.CreatedAt, &item.DeletionPending, &item.DeletionRequiresAction); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
func (s *Service) CreateConversation(ctx context.Context, title string) (Conversation, error) {
	if !validName(title) || len(title) > 80 {
		return Conversation{}, ErrInvalid
	}
	item := Conversation{ID: state.Random(), Title: title, CreatedAt: time.Now().Unix()}
	err := s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		if err := state.RequireAdmission(tx); err != nil {
			return errors.Join(ErrConflict, err)
		}
		var count int
		if err := tx.QueryRow("SELECT count(*) FROM conversations").Scan(&count); err != nil {
			return err
		}
		if count >= 100 {
			return ErrConflict
		}
		_, err := tx.Exec("INSERT INTO conversations VALUES(?,?,?)", item.ID, item.Title, item.CreatedAt)
		return err
	})
	return item, err
}
func (s *Service) generation(ctx context.Context, id string) (Generation, error) {
	var g Generation
	var reference string
	err := s.Store.DB.QueryRowContext(ctx, "SELECT id,conversation_id,prompt,state,operation_id,created_at,updated_at FROM generations WHERE id=?", id).Scan(&g.ID, &g.ConversationID, &reference, &g.State, &g.OperationID, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		return g, err
	}
	err = json.Unmarshal([]byte(reference), &g.reference)
	return g, err
}
func (s *Service) readPrompt(ctx context.Context, instance string, reference objectReference) ([]guestproto.Message, error) {
	if !guestproto.ValidID(reference.ID) || reference.Size < 1 || reference.Size > 16<<10 || !hashPattern.MatchString(reference.SHA256) {
		return nil, ErrConflict
	}
	response, err := s.call(ctx, instance, guestproto.Request{Operation: "download", ObjectID: reference.ID})
	if err != nil {
		return nil, err
	}
	if int64(len(response.Data)) != reference.Size || sum(response.Data) != reference.SHA256 {
		return nil, ErrConflict
	}
	var messages []guestproto.Message
	if err = json.Unmarshal(response.Data, &messages); err != nil {
		return nil, err
	}
	if len(messages) == 0 || len(messages) > 17 {
		return nil, ErrConflict
	}
	return messages, nil
}
func (s *Service) Generation(ctx context.Context, id string) (Generation, error) {
	g, err := s.generation(ctx, id)
	if err != nil {
		return g, err
	}
	if g.State == "staging" {
		return g, nil
	}
	instance, err := s.app(ctx, "ai")
	if err != nil {
		return g, err
	}
	messages, err := s.readPrompt(ctx, instance, g.reference)
	if err != nil {
		return g, err
	}
	g.Prompt = messages[len(messages)-1].Content
	if g.State == "queued" || g.State == "staging" {
		return g, nil
	}
	result, err := s.call(ctx, instance, guestproto.Request{Operation: "result", ObjectID: id})
	if err != nil {
		if result.Error == "NOT_FOUND" && (g.State == "cancelled" || g.State == "failed" || g.State == "interrupted") {
			return g, nil
		}
		return g, err
	}
	g.Output = result.Text
	return g, nil
}
func (s *Service) History(ctx context.Context, id string) ([]Generation, error) {
	rows, err := s.Store.DB.QueryContext(ctx, "SELECT id FROM generations WHERE conversation_id=? ORDER BY created_at,rowid LIMIT 100", id)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	items := []Generation{}
	for _, id := range ids {
		item, err := s.Generation(ctx, id)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}
func (s *Service) CreateGeneration(ctx context.Context, device, key, conversation, prompt string) (Generation, error) {
	unlock := s.lock(conversation)
	defer unlock()
	if !guestproto.ValidID(conversation) || len(strings.TrimSpace(prompt)) == 0 || len(prompt) > 2048 || !utf8.ValidString(prompt) {
		return Generation{}, ErrInvalid
	}
	instance, err := s.app(ctx, "ai")
	if err != nil {
		return Generation{}, err
	}
	history, err := s.History(ctx, conversation)
	if err != nil {
		return Generation{}, err
	}
	messages := []guestproto.Message{{Role: "user", Content: prompt}}
	size := len(prompt)
	for i := len(history) - 1; i >= 0 && len(messages) < 15; i-- {
		g := history[i]
		if g.State != "succeeded" {
			continue
		}
		if size+len(g.Prompt)+len(g.Output) > 8<<10 {
			break
		}
		messages = append([]guestproto.Message{{Role: "user", Content: g.Prompt}, {Role: "assistant", Content: g.Output}}, messages...)
		size += len(g.Prompt) + len(g.Output)
	}
	data, err := json.Marshal(messages)
	if err != nil {
		return Generation{}, err
	}
	reference := objectReference{ID: state.Random(), Size: int64(len(data)), SHA256: sum(data)}
	generationID := state.Random()
	err = s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		deleting, err := conversationDeletionPending(tx, conversation)
		if err != nil {
			return err
		}
		if deleting {
			return ErrConflict
		}
		op, replay, err := operation(tx, device, key, "generation", map[string]string{"conversationId": conversation, "prompt": prompt})
		if err != nil {
			return err
		}
		if replay {
			return tx.QueryRow("SELECT id FROM generations WHERE operation_id=?", op.ID).Scan(&generationID)
		}
		if err := state.RequireAdmission(tx); err != nil {
			return errors.Join(ErrConflict, err)
		}

		var active, total, conversationCount int
		if err = tx.QueryRow("SELECT count(*) FROM generations WHERE state IN('staging','queued','running','cancelling')").Scan(&active); err != nil {
			return err
		}
		if err = tx.QueryRow("SELECT count(*) FROM generations").Scan(&total); err != nil {
			return err
		}
		if err = tx.QueryRow("SELECT count(*) FROM generations WHERE conversation_id=?", conversation).Scan(&conversationCount); err != nil {
			return err
		}
		if active >= 8 || total >= 1000 || conversationCount >= 100 {
			return ErrConflict
		}
		var busy int
		if err = tx.QueryRow("SELECT count(*) FROM generations WHERE conversation_id=? AND state IN('staging','queued','running','cancelling')", conversation).Scan(&busy); err != nil {
			return err
		}
		if busy > 0 {
			return ErrConflict
		}
		// Only the bounded object reference persists in management state. Chat
		// content is stored on the AI guest's data disk, not in this database.
		encoded, _ := json.Marshal(reference)
		now := time.Now().Unix()
		if _, err = tx.Exec("INSERT INTO generations(id,device_id,conversation_id,prompt,state,operation_id,created_at,updated_at) VALUES(?,?,?,?,'staging',?,?,?)", generationID, device, conversation, string(encoded), op.ID, now, now); err != nil {
			return err
		}
		_, err = tx.Exec("UPDATE operations SET result='{}' WHERE id=?", op.ID)
		return err
	})
	if err != nil {
		return Generation{}, err
	}
	g, err := s.generation(ctx, generationID)
	if err != nil {
		return g, err
	}
	if g.State != "staging" {
		return s.Generation(ctx, generationID)
	}
	// A retried staging request uses its original content reference. If history
	// changed in between, reject rather than filling it with different bytes.
	reference = g.reference
	if reference.Size != int64(len(data)) || reference.SHA256 != sum(data) {
		return g, ErrConflict
	}
	current, statErr := s.call(ctx, instance, guestproto.Request{Operation: "stat", ObjectID: reference.ID})
	if statErr != nil || current.Size == 0 {
		if _, err = s.call(ctx, instance, guestproto.Request{Operation: "upload", ObjectID: reference.ID, Data: data, Size: reference.Size, SHA256: reference.SHA256}); err != nil {
			return g, err
		}
	}
	final, err := s.call(ctx, instance, guestproto.Request{Operation: "finalize", ObjectID: reference.ID, Size: reference.Size, SHA256: reference.SHA256})
	if err != nil {
		return g, err
	}
	if final.Size != reference.Size || final.SHA256 != reference.SHA256 {
		return g, ErrConflict
	}
	if _, err = s.Store.DB.ExecContext(ctx, "UPDATE generations SET state='queued',updated_at=? WHERE id=? AND state='staging'", time.Now().Unix(), generationID); err != nil {
		return g, err
	}
	if err = s.Store.DB.QueryRowContext(ctx, "SELECT state FROM generations WHERE id=?", generationID).Scan(&g.State); err != nil {
		return g, err
	}
	g.Prompt = prompt
	return g, nil
}
func (s *Service) CancelGeneration(ctx context.Context, id string) error {
	g, err := s.generation(ctx, id)
	if err != nil {
		return err
	}
	var phase string
	err = s.Store.DB.QueryRowContext(ctx, "UPDATE generations SET state=CASE WHEN state IN('staging','queued') THEN 'cancelled' ELSE 'cancelling' END,updated_at=? WHERE id=? AND state IN('staging','queued','running','cancelling') RETURNING state", time.Now().Unix(), id).Scan(&phase)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if phase == "cancelled" {
		return s.endGeneration(ctx, g, "cancelled", "CANCELLED")
	}
	return nil
}
func (s *Service) processGeneration(ctx context.Context) error {
	if s.Backend == nil {
		return nil
	}
	var id string
	err := s.Store.DB.QueryRowContext(ctx, "SELECT id FROM generations WHERE state='queued' ORDER BY created_at,rowid LIMIT 1").Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	g, err := s.generation(ctx, id)
	if err != nil {
		return err
	}
	instance, err := s.app(ctx, "ai")
	if err != nil {
		return s.endGeneration(ctx, g, "failed", "AI_UNAVAILABLE")
	}
	var permitted int
	if err = s.Store.DB.QueryRowContext(ctx, "SELECT count(*) FROM devices WHERE id=(SELECT device_id FROM generations WHERE id=?) AND revoked_at IS NULL AND EXISTS(SELECT 1 FROM json_each(capabilities) WHERE value='ai')", id).Scan(&permitted); err != nil {
		return err
	}
	if permitted != 1 {
		return s.endGeneration(ctx, g, "failed", "AUTHORIZATION_EXPIRED")
	}
	claimed := false
	err = s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		result, err := tx.Exec("UPDATE generations SET state='running',updated_at=? WHERE id=? AND state='queued'", time.Now().Unix(), id)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return nil
		}
		claimed = true
		_, err = tx.Exec("UPDATE operations SET state='executing',updated_at=? WHERE id=?", time.Now().Unix(), g.OperationID)
		return err
	})
	if err != nil || !claimed {
		return err
	}

	generationCtx, cancel := context.WithTimeout(ctx, 150*time.Second)
	defer cancel()
	if _, err = s.call(generationCtx, instance, guestproto.Request{Operation: "generate", ObjectID: id, InputID: g.reference.ID}); err != nil {
		return s.endGeneration(ctx, g, "failed", "GENERATION_FAILED")
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-generationCtx.Done():
			cleanup, c := context.WithTimeout(context.Background(), 10*time.Second)
			_, _ = s.call(cleanup, instance, guestproto.Request{Operation: "cancel", ObjectID: id})
			err = s.endGeneration(cleanup, g, "interrupted", "GENERATION_INTERRUPTED")
			c()
			return err
		case <-ticker.C:
			current, e := s.generation(generationCtx, id)
			if e != nil {
				return e
			}
			if current.State == "cancelling" {
				response, e := s.call(generationCtx, instance, guestproto.Request{Operation: "cancel", ObjectID: id})
				if e != nil || response.State != "cancelled" {
					return s.endGeneration(generationCtx, g, "interrupted", "CANCEL_REQUIRES_ATTENTION")
				}
				return s.endGeneration(generationCtx, g, "cancelled", "CANCELLED")
			}
			response, e := s.call(generationCtx, instance, guestproto.Request{Operation: "result", ObjectID: id})
			if e != nil {
				return s.endGeneration(generationCtx, g, "interrupted", "AI_UNAVAILABLE")
			}
			if response.State == "running" {
				continue
			}
			if response.State == "succeeded" {
				return s.endGeneration(generationCtx, g, "succeeded", "")
			}
			return s.endGeneration(generationCtx, g, "failed", "GENERATION_FAILED")
		}
	}
}
func (s *Service) endGeneration(ctx context.Context, g Generation, phase, code string) error {
	return s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		var device, current string
		if err := tx.QueryRow("SELECT state FROM generations WHERE id=?", g.ID).Scan(&current); err != nil {
			return err
		}
		if current == "cancelling" && phase == "succeeded" {
			phase = "cancelled"
			code = "CANCELLED"
		}
		if err := tx.QueryRow("SELECT device_id FROM generations WHERE id=?", g.ID).Scan(&device); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE generations SET state=?,updated_at=? WHERE id=?", phase, time.Now().Unix(), g.ID); err != nil {
			return err
		}
		opState := "failed"
		if phase == "succeeded" {
			opState = "succeeded"
		}
		if phase == "interrupted" {
			opState = "requires-action"
		}
		if _, err := tx.Exec("UPDATE operations SET state=?,result=?,updated_at=? WHERE id=?", opState, `{"code":"`+code+`"}`, time.Now().Unix(), g.OperationID); err != nil {
			return err
		}
		return state.Event(tx, device, "generation."+phase, g.ID, map[string]string{"code": code})
	})
}

func (s *Service) DeleteConversation(ctx context.Context, device, id string) error {
	return s.deleteConversation(ctx, device, id, "")
}

func (s *Service) deleteConversation(ctx context.Context, device, id, operationID string) error {
	unlock := s.lock(id)
	defer unlock()
	var active int
	if err := s.Store.DB.QueryRowContext(ctx, "SELECT count(*) FROM generations WHERE conversation_id=? AND state IN('staging','queued','running','cancelling')", id).Scan(&active); err != nil {
		return err
	}
	if active > 0 {
		return ErrConflict
	}
	rows, err := s.Store.DB.QueryContext(ctx, "SELECT id,prompt FROM generations WHERE conversation_id=? ORDER BY id", id)
	if err != nil {
		return err
	}
	objects := [][2]string{}
	for rows.Next() {
		var generation, encoded string
		if err = rows.Scan(&generation, &encoded); err != nil {
			rows.Close()
			return err
		}
		var reference objectReference
		if err = json.Unmarshal([]byte(encoded), &reference); err != nil {
			rows.Close()
			return err
		}
		if !guestproto.ValidID(generation) || !guestproto.ValidID(reference.ID) {
			rows.Close()
			return ErrConflict
		}
		objects = append(objects, [2]string{generation, reference.ID})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	instance := ""
	if len(objects) > 0 {
		instance, err = s.app(ctx, "ai")
		if err != nil {
			return err
		}
	}
	for _, pair := range objects {
		for _, object := range pair {
			if _, err = s.call(ctx, instance, guestproto.Request{Operation: "delete", ObjectID: object}); err != nil {
				return err
			}
		}
		if operationID != "" {
			// Removing only acknowledged generation metadata is the durable
			// checkpoint. Restart scans remaining rows, never a guessed cursor.
			result, err := s.Store.DB.ExecContext(ctx, "DELETE FROM generations WHERE id=? AND conversation_id=? AND EXISTS(SELECT 1 FROM operations WHERE id=? AND kind='conversation.delete' AND state='pending' AND json_extract(result,'$.conversationId')=?)", pair[0], id, operationID, id)
			if err != nil {
				return err
			}
			count, err := result.RowsAffected()
			if err != nil {
				return err
			}
			if count != 1 {
				return ErrConflict
			}
		}
	}
	return s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("DELETE FROM generations WHERE conversation_id=?", id); err != nil {
			return err
		}
		result, err := tx.Exec("DELETE FROM conversations WHERE id=?", id)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrInvalid
		}
		if operationID != "" {
			result, err := tx.Exec("UPDATE operations SET state='succeeded',result='{\"deleted\":true}',updated_at=? WHERE id=? AND kind='conversation.delete' AND state='pending'", time.Now().Unix(), operationID)
			if err != nil {
				return err
			}
			count, err := result.RowsAffected()
			if err != nil {
				return err
			}
			if count != 1 {
				return ErrConflict
			}
		}
		return state.Event(tx, device, "conversation.deleted", id, map[string]any{})
	})
}
func (s *Service) reconcileGenerations(ctx context.Context) error {
	rows, err := s.Store.DB.QueryContext(ctx, "SELECT id FROM generations WHERE state IN('running','cancelling')")
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		g, err := s.generation(ctx, id)
		if err != nil {
			return err
		}
		if instance, e := s.app(ctx, "ai"); e == nil {
			response, e := s.call(ctx, instance, guestproto.Request{Operation: "cancel", ObjectID: id})
			if e != nil || response.State != "cancelled" && response.State != "succeeded" && response.State != "failed" && response.State != "interrupted" {
				return ErrUnavailable
			}
		}
		if err = s.endGeneration(ctx, g, "interrupted", "CONTROLLER_RESTARTED"); err != nil {
			return err
		}
	}
	return nil
}
