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
	if !validName(name) || size < 0 || size > MaxFileBytes || !hashPattern.MatchString(hash) {
		return Transfer{}, ErrInvalid
	}
	if _, err := s.app(ctx, "files"); err != nil {
		return Transfer{}, err
	}
	transfer := Transfer{ID: state.Random(), Name: name, Size: size, SHA256: hash, State: "uploading", ExpiresAt: time.Now().Add(24 * time.Hour).Unix()}
	err := s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		var reserved int64
		var count int
		if err := tx.QueryRow("SELECT coalesce(sum(size),0),count(*) FROM transfers WHERE state IN('uploading','verifying')").Scan(&reserved, &count); err != nil {
			return err
		}
		var activeJobs int
		if err := tx.QueryRow("SELECT count(*) FROM jobs WHERE state IN('queued','preparing','running','finalizing','cancelling')").Scan(&activeJobs); err != nil {
			return err
		}
		var used int64
		if err := tx.QueryRow("SELECT coalesce(sum(size),0) FROM files").Scan(&used); err != nil {
			return err
		}
		if count >= 16 || used+reserved+size+int64(activeJobs)*jobOutputBudget > StorageQuota {
			return ErrConflict
		}
		_, err := tx.Exec("INSERT INTO transfers(id,device_id,name,size,sha256,state,created_at,expires_at) VALUES(?,?,?,?,?,'uploading',?,?)", transfer.ID, device, name, size, hash, time.Now().Unix(), transfer.ExpiresAt)
		return err
	})
	return transfer, err
}
func (s *Service) Transfer(ctx context.Context, device, id string) (Transfer, error) {
	var t Transfer
	err := s.Store.DB.QueryRowContext(ctx, "SELECT id,name,size,sha256,offset,state,expires_at FROM transfers WHERE id=? AND device_id=?", id, device).Scan(&t.ID, &t.Name, &t.Size, &t.SHA256, &t.Offset, &t.State, &t.ExpiresAt)
	return t, err
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
	if response.Offset < offset+int64(len(data)) || response.Offset > t.Size || response.Offset > max(t.Offset, offset+int64(len(data))) {
		return t, ErrConflict
	}
	_, err = s.Store.DB.ExecContext(ctx, "UPDATE transfers SET offset=? WHERE id=?", response.Offset, id)
	if err != nil {
		return t, err
	}
	t.Offset = response.Offset
	return t, nil
}
func (s *Service) Finalize(ctx context.Context, device, id string) (File, error) {
	unlock := s.lock(id)
	defer unlock()
	t, err := s.Transfer(ctx, device, id)
	if err != nil {
		return File{}, err
	}
	if t.State == "ready" {
		return s.File(ctx, id)
	}
	if t.State != "uploading" && t.State != "verifying" || t.Offset != t.Size || t.ExpiresAt <= time.Now().Unix() {
		return File{}, ErrConflict
	}
	instance, err := s.app(ctx, "files")
	if err != nil {
		return File{}, err
	}
	if _, err = s.Store.DB.ExecContext(ctx, "UPDATE transfers SET state='verifying' WHERE id=?", id); err != nil {
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
	err := s.Store.DB.QueryRowContext(ctx, "SELECT id,name,size,sha256,created_at,trash_until FROM files WHERE id=?", id).Scan(&f.ID, &f.Name, &f.Size, &f.SHA256, &f.CreatedAt, &f.TrashUntil)
	return f, err
}
func (s *Service) Files(ctx context.Context) ([]File, error) {
	rows, err := s.Store.DB.QueryContext(ctx, "SELECT id,name,size,sha256,created_at,trash_until FROM files ORDER BY created_at DESC LIMIT 10000")
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
func (s *Service) ChangeFile(ctx context.Context, device, id, action, name string) error {
	if !guestproto.ValidID(id) {
		return ErrInvalid
	}
	return s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		var result sql.Result
		var err error
		switch action {
		case "rename":
			if !validName(name) {
				return ErrInvalid
			}
			result, err = tx.Exec("UPDATE files SET name=? WHERE id=?", name, id)
		case "trash":
			result, err = tx.Exec("UPDATE files SET trash_until=? WHERE id=? AND trash_until IS NULL", time.Now().Add(7*24*time.Hour).Unix(), id)
		case "restore":
			result, err = tx.Exec("UPDATE files SET trash_until=NULL WHERE id=?", id)
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
