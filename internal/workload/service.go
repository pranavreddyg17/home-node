// Package workload owns durable user intent and data metadata. All content
// lives behind the transfer service inside approved guests.
package workload

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash/fnv"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"github.com/pranavreddyg17/home-node/internal/state"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

const MaxFileBytes int64 = 1 << 30
const StorageQuota int64 = 12 << 30

var ErrUnavailable = errors.New("start the required workload on a qualified host")
var ErrConflict = errors.New("operation conflicts with current state")
var ErrCapacity = errors.New("workload storage capacity unavailable")
var ErrInvalid = errors.New("invalid workload request")
var hashPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type Backend interface {
	Apply(context.Context, supervisor.Request) (supervisor.Instance, error)
	Call(context.Context, string, guestproto.Request) (guestproto.Response, error)
}
type Service struct {
	Store            *state.Store
	Backend          Backend
	PolicyGeneration int64
	locks            [64]sync.Mutex
}

func New(store *state.Store, backend Backend, generation int64) *Service {
	return &Service{Store: store, Backend: backend, PolicyGeneration: generation}
}
func (s *Service) lock(id string) func() {
	h := fnv.New32a()
	_, _ = h.Write([]byte(id))
	lock := &s.locks[h.Sum32()%64]
	lock.Lock()
	return lock.Unlock
}
func validName(name string) bool {
	return len(strings.TrimSpace(name)) > 0 && len(name) <= 200 && !strings.ContainsAny(name, "\x00\r\n/\\")
}
func sum(data []byte) string { hash := sha256.Sum256(data); return hex.EncodeToString(hash[:]) }
func (s *Service) call(ctx context.Context, instance string, r guestproto.Request) (guestproto.Response, error) {
	if s.Backend == nil {
		return guestproto.Response{}, ErrUnavailable
	}
	r.Version = 1
	r.RequestID = state.Random()
	response, err := s.Backend.Call(ctx, instance, r)
	if err != nil {
		return response, err
	}
	if response.Error != "" {
		if response.Error == "CAPACITY_UNAVAILABLE" {
			return response, ErrCapacity
		}
		return response, ErrConflict
	}
	return response, nil
}
func (s *Service) app(ctx context.Context, name string) (string, error) {
	var id string
	if s.Backend == nil {
		return "", ErrUnavailable
	}
	err := s.Store.DB.QueryRowContext(ctx, "SELECT instance_id FROM apps WHERE workload=? AND state='running'", name).Scan(&id)
	if err != nil {
		return "", ErrUnavailable
	}
	return id, nil
}

type Transfer struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256"`
	Offset    int64  `json:"offset"`
	State     string `json:"state"`
	ExpiresAt int64  `json:"expiresAt"`
}
type File struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	SHA256     string `json:"sha256"`
	CreatedAt  int64  `json:"createdAt"`
	TrashUntil *int64 `json:"trashUntil"`
}

func (s *Service) CreateTransfer(ctx context.Context, device, name string, size int64, hash string) (Transfer, error) {
	return s.createTransfer(ctx, device, name, size, hash, "")
}

func (s *Service) CreateTransferWithKey(ctx context.Context, device, name string, size int64, hash, key string) (Transfer, error) {
	if len(key) < 16 || len(key) > 128 {
		return Transfer{}, ErrInvalid
	}
	return s.createTransfer(ctx, device, name, size, hash, key)
}

func (s *Service) createTransfer(ctx context.Context, device, name string, size int64, hash, key string) (Transfer, error) {
	if !validName(name) || size < 0 || size > MaxFileBytes || !hashPattern.MatchString(hash) {
		return Transfer{}, ErrInvalid
	}
	_, appErr := s.app(ctx, "files")
	transfer := Transfer{ID: state.Random(), Name: name, Size: size, SHA256: hash, State: "uploading", ExpiresAt: time.Now().Add(24 * time.Hour).Unix()}
	err := s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		var creation Operation
		if key != "" {
			var replay bool
			var err error
			creation, replay, err = operation(tx, device, key, "transfer.create", struct {
				Name   string
				Size   int64
				SHA256 string
			}{name, size, hash})
			if err != nil {
				return err
			}
			if replay {
				var result struct {
					TransferID string `json:"transferId"`
				}
				if creation.State != "succeeded" || json.Unmarshal(creation.Result, &result) != nil || !guestproto.ValidID(result.TransferID) {
					return ErrConflict
				}
				return tx.QueryRow("SELECT id,name,size,sha256,offset,state,expires_at FROM transfers WHERE id=? AND device_id=?", result.TransferID, device).Scan(&transfer.ID, &transfer.Name, &transfer.Size, &transfer.SHA256, &transfer.Offset, &transfer.State, &transfer.ExpiresAt)
			}
		}
		if err := state.RequireAdmission(tx); err != nil {
			return errors.Join(ErrConflict, err)
		}
		if appErr != nil {
			return appErr
		}
		usage, err := filesStorage(ctx, tx)
		if err != nil {
			return err
		}
		if usage.activeTransfers >= 16 {
			return ErrConflict
		}
		if usage.ChargedBytes+size > StorageQuota || usage.AvailableFileEntries < 1 {
			return ErrCapacity
		}
		_, err = tx.Exec("INSERT INTO transfers(id,device_id,name,size,sha256,state,created_at,expires_at) VALUES(?,?,?,?,?,'uploading',?,?)", transfer.ID, device, name, size, hash, time.Now().Unix(), transfer.ExpiresAt)
		if err == nil && key != "" {
			result, encodeErr := json.Marshal(map[string]string{"transferId": transfer.ID})
			if encodeErr != nil {
				return encodeErr
			}
			_, err = tx.Exec("UPDATE operations SET state='succeeded',result=?,updated_at=? WHERE id=?", string(result), time.Now().Unix(), creation.ID)
		}
		return err
	})
	return transfer, err
}
func (s *Service) Transfer(ctx context.Context, device, id string) (Transfer, error) {
	var t Transfer
	err := s.Store.DB.QueryRowContext(ctx, "SELECT id,name,size,sha256,offset,state,expires_at FROM transfers WHERE id=? AND device_id=?", id, device).Scan(&t.ID, &t.Name, &t.Size, &t.SHA256, &t.Offset, &t.State, &t.ExpiresAt)
	return t, err
}

// PendingTransfers is scoped to the same device as individual transfer access.
// Terminal records remain in the journal but do not need owner intervention.
func (s *Service) PendingTransfers(ctx context.Context, device string) ([]Transfer, error) {
	rows, err := s.Store.DB.QueryContext(ctx, `SELECT id,name,size,sha256,offset,state,expires_at
FROM transfers WHERE device_id=? AND state IN('uploading','verifying','cancelling')
ORDER BY created_at,id LIMIT 16`, device)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Transfer{}
	for rows.Next() {
		var item Transfer
		if err := rows.Scan(&item.ID, &item.Name, &item.Size, &item.SHA256, &item.Offset, &item.State, &item.ExpiresAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
func (s *Service) Upload(ctx context.Context, device, id string, offset int64, data []byte, hash string) (Transfer, error) {
	unlock := s.lock(id)
	defer unlock()
	t, err := s.Transfer(ctx, device, id)
	if err != nil {
		return t, err
	}
	if t.State != "uploading" || t.ExpiresAt <= time.Now().Unix() || offset < 0 || offset > t.Offset || len(data) == 0 || len(data) > guestproto.ChunkSize || offset+int64(len(data)) > t.Size || sum(data) != hash {
		return t, ErrConflict
	}
	instance, err := s.app(ctx, "files")
	if err != nil {
		return t, err
	}
	response, err := s.call(ctx, instance, guestproto.Request{Operation: "upload", ObjectID: id, Offset: offset, Size: t.Size, Data: data, SHA256: hash})
	if err != nil {
		return t, err
	}
	if response.Offset != max(t.Offset, offset+int64(len(data))) || response.Offset > t.Size {
		return t, ErrConflict
	}
	_, err = s.Store.DB.ExecContext(ctx, "UPDATE transfers SET offset=? WHERE id=?", response.Offset, id)
	if err != nil {
		return t, err
	}
	t.Offset = response.Offset
	return t, nil
}

// Finalize is the trusted component entry point. Public controller requests use
// FinalizeWithAuthority to bind publication to a live session transaction.
func (s *Service) Finalize(ctx context.Context, device, id string) (File, error) {
	return s.finalize(ctx, device, id, nil)
}

// FinalizeWithAuthority binds public publication to current authority in the
// same transaction that makes verified guest bytes visible as a saved file.
func (s *Service) FinalizeWithAuthority(ctx context.Context, device, id string, authorize func(*sql.Tx) error) (File, error) {
	if authorize == nil {
		return File{}, ErrInvalid
	}
	return s.finalize(ctx, device, id, authorize)
}

func (s *Service) finalize(ctx context.Context, device, id string, authorize func(*sql.Tx) error) (File, error) {
	unlock := s.lock(id)
	defer unlock()
	t, err := s.Transfer(ctx, device, id)
	if err != nil {
		return File{}, err
	}
	if t.State == "ready" {
		if authorize != nil {
			if err := s.Store.Transaction(ctx, authorize); err != nil {
				return File{}, err
			}
		}
		return s.File(ctx, id)
	}
	if t.State != "uploading" && t.State != "verifying" || t.Offset != t.Size || t.ExpiresAt <= time.Now().Unix() {
		return File{}, ErrConflict
	}
	instance, err := s.app(ctx, "files")
	if err != nil {
		return File{}, err
	}
	err = s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		if authorize != nil {
			if err := authorize(tx); err != nil {
				return err
			}
		}
		_, err := tx.Exec("UPDATE transfers SET state='verifying' WHERE id=?", id)
		return err
	})
	if err != nil {
		return File{}, err
	}
	response, err := s.call(ctx, instance, guestproto.Request{Operation: "finalize", ObjectID: id, Size: t.Size, SHA256: t.SHA256})
	if err != nil {
		return File{}, err
	}
	if response.Size != t.Size || response.SHA256 != t.SHA256 {
		return File{}, ErrConflict
	}
	err = s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		if authorize != nil {
			if err := authorize(tx); err != nil {
				return err
			}
		}
		if _, err := tx.Exec("INSERT INTO files(id,transfer_id,name,size,sha256,created_at) VALUES(?,?,?,?,?,?) ON CONFLICT(id) DO NOTHING", id, id, t.Name, t.Size, t.SHA256, time.Now().Unix()); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE transfers SET state='ready' WHERE id=?", id); err != nil {
			return err
		}
		return state.Event(tx, device, "file.created", id, map[string]any{"size": t.Size})
	})
	if err != nil {
		return File{}, err
	}
	return s.File(ctx, id)
}
func (s *Service) File(ctx context.Context, id string) (File, error) {
	var f File
	err := s.Store.DB.QueryRowContext(ctx, "SELECT id,name,size,sha256,created_at,trash_until FROM files WHERE id=? AND (trash_until IS NULL OR trash_until>=0)", id).Scan(&f.ID, &f.Name, &f.Size, &f.SHA256, &f.CreatedAt, &f.TrashUntil)
	return f, err
}
func (s *Service) Files(ctx context.Context) ([]File, error) {
	rows, err := s.Store.DB.QueryContext(ctx, "SELECT id,name,size,sha256,created_at,trash_until FROM files WHERE trash_until IS NULL OR trash_until>=0 ORDER BY created_at DESC LIMIT ?", FileEntryLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	files := []File{}
	for rows.Next() {
		var f File
		if err = rows.Scan(&f.ID, &f.Name, &f.Size, &f.SHA256, &f.CreatedAt, &f.TrashUntil); err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	return files, rows.Err()
}

// ExportableFiles refuses an oversized manifest rather than silently exporting
// the truncated ordinary file-list page. Trash is deliberately outside scope.
func (s *Service) ExportableFiles(ctx context.Context) ([]File, error) {
	rows, err := s.Store.DB.QueryContext(ctx, "SELECT id,name,size,sha256,created_at,trash_until FROM files WHERE trash_until IS NULL ORDER BY created_at,id LIMIT ?", FileEntryLimit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	files := []File{}
	for rows.Next() {
		var f File
		if err := rows.Scan(&f.ID, &f.Name, &f.Size, &f.SHA256, &f.CreatedAt, &f.TrashUntil); err != nil {
			return nil, err
		}
		files = append(files, f)
		if len(files) > FileEntryLimit {
			return nil, ErrCapacity
		}
	}
	return files, rows.Err()
}
func (s *Service) ChangeFile(ctx context.Context, device, id, action, name string) error {
	if !guestproto.ValidID(id) {
		return ErrInvalid
	}
	unlock := s.lock(id)
	defer unlock()
	return s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		if err := state.RequireAdmission(tx); err != nil {
			return errors.Join(ErrConflict, err)
		}
		var result sql.Result
		var err error
		switch action {
		case "rename":
			if !validName(name) {
				return ErrInvalid
			}
			result, err = tx.Exec("UPDATE files SET name=? WHERE id=? AND (trash_until IS NULL OR trash_until>?)", name, id, time.Now().Unix())
		case "trash":
			result, err = tx.Exec("UPDATE files SET trash_until=? WHERE id=? AND trash_until IS NULL", time.Now().Add(7*24*time.Hour).Unix(), id)
		case "restore":
			result, err = tx.Exec("UPDATE files SET trash_until=NULL WHERE id=? AND trash_until>?", id, time.Now().Unix())
		default:
			return ErrInvalid
		}
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrConflict
		}
		return state.Event(tx, device, "file."+action, id, map[string]any{})
	})
}
func (s *Service) Download(ctx context.Context, id string, offset int64) ([]byte, error) {
	f, err := s.File(ctx, id)
	if err != nil {
		return nil, err
	}
	if f.TrashUntil != nil || offset < 0 || offset > f.Size {
		return nil, ErrConflict
	}
	instance, err := s.app(ctx, "files")
	if err != nil {
		return nil, err
	}
	response, err := s.call(ctx, instance, guestproto.Request{Operation: "download", ObjectID: id, Offset: offset})
	if err != nil {
		return nil, err
	}
	if len(response.Data) > guestproto.ChunkSize || int64(len(response.Data)) != min(int64(guestproto.ChunkSize), f.Size-offset) || response.SHA256 != sum(response.Data) {
		return nil, ErrConflict
	}
	return response.Data, nil
}

type Operation struct {
	ID        string          `json:"id"`
	Kind      string          `json:"kind"`
	State     string          `json:"state"`
	Result    json.RawMessage `json:"result"`
	CreatedAt int64           `json:"createdAt"`
	UpdatedAt int64           `json:"updatedAt"`
}

func operation(tx *sql.Tx, device, key, kind string, body any) (Operation, bool, error) {
	if len(key) < 16 || len(key) > 128 {
		return Operation{}, false, ErrInvalid
	}
	data, err := json.Marshal(body)
	if err != nil {
		return Operation{}, false, err
	}
	hash := sum(data)
	var existingHash string
	var o Operation
	var result string
	err = tx.QueryRow("SELECT id,kind,state,result,created_at,updated_at,request_hash FROM operations WHERE device_id=? AND idempotency_key=?", device, key).Scan(&o.ID, &o.Kind, &o.State, &result, &o.CreatedAt, &o.UpdatedAt, &existingHash)
	if err == nil {
		if existingHash != hash || o.Kind != kind {
			return o, false, ErrConflict
		}
		o.Result = json.RawMessage(result)
		return o, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return o, false, err
	}
	resultData := data
	if kind == "generation" {
		resultData = []byte("{}")
	}
	o = Operation{ID: state.Random(), Kind: kind, State: "pending", Result: json.RawMessage(resultData), CreatedAt: time.Now().Unix(), UpdatedAt: time.Now().Unix()}
	_, err = tx.Exec("INSERT INTO operations(id,device_id,kind,state,request_hash,idempotency_key,result,created_at,updated_at) VALUES(?,?,?,'pending',?,?,?,?,?)", o.ID, device, kind, hash, key, string(resultData), o.CreatedAt, o.UpdatedAt)
	return o, false, err
}
func (s *Service) Operation(ctx context.Context, device, id string) (Operation, error) {
	var o Operation
	var data string
	err := s.Store.DB.QueryRowContext(ctx, "SELECT id,kind,state,result,created_at,updated_at FROM operations WHERE device_id=? AND id=?", device, id).Scan(&o.ID, &o.Kind, &o.State, &data, &o.CreatedAt, &o.UpdatedAt)
	o.Result = json.RawMessage(data)
	return o, err
}
func (s *Service) finish(ctx context.Context, id, phase string, result any) error {
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		var device string
		if err := tx.QueryRow("SELECT device_id FROM operations WHERE id=?", id).Scan(&device); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE operations SET state=?,result=?,updated_at=? WHERE id=?", phase, string(data), time.Now().Unix(), id); err != nil {
			return err
		}
		return state.Event(tx, device, "operation."+phase, id, result)
	})
}
