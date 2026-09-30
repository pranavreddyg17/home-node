package workload

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"github.com/pranavreddyg17/home-node/internal/state"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

// MaxJobOutputBytes bounds result copying and its durable storage reservation.
const MaxJobOutputBytes int64 = 4 << 30

type Job struct {
	CleanupPending bool    `json:"cleanupPending"`
	StartRequested bool    `json:"-"`
	ID             string  `json:"id"`
	AttemptID      string  `json:"attemptId"`
	InputID        string  `json:"inputId"`
	Preset         string  `json:"preset"`
	State          string  `json:"state"`
	InstanceID     string  `json:"-"`
	OperationID    string  `json:"operationId"`
	OutputID       *string `json:"outputId"`
	ErrorCode      *string `json:"errorCode"`
	CreatedAt      int64   `json:"createdAt"`
	UpdatedAt      int64   `json:"updatedAt"`
}

const jobColumns = "group_id,id,input_id,preset,state,instance_id,operation_id,output_id,error_code,created_at,updated_at,start_requested,EXISTS(SELECT 1 FROM settings WHERE key='job.cleanup.'||jobs.id AND CASE WHEN json_valid(value) THEN COALESCE(json_extract(value,'$.state'),'pending') ELSE 'pending' END<>'done' AND jobs.state IN('succeeded','failed','cancelled','interrupted'))"

func scanJob(row interface{ Scan(...any) error }) (Job, error) {
	var j Job
	err := row.Scan(&j.ID, &j.AttemptID, &j.InputID, &j.Preset, &j.State, &j.InstanceID, &j.OperationID, &j.OutputID, &j.ErrorCode, &j.CreatedAt, &j.UpdatedAt, &j.StartRequested, &j.CleanupPending)
	return j, err
}
func (s *Service) Jobs(ctx context.Context) ([]Job, error) {
	rows, err := s.Store.DB.QueryContext(ctx, "SELECT "+jobColumns+" FROM jobs ORDER BY created_at DESC,rowid DESC LIMIT 1000")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Job{}
	for rows.Next() {
		item, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
func (s *Service) attempt(ctx context.Context, id string) (Job, error) {
	return scanJob(s.Store.DB.QueryRowContext(ctx, "SELECT "+jobColumns+" FROM jobs WHERE id=?", id))
}
func (s *Service) CreateJob(ctx context.Context, device, key, input, preset, retryGroup string) (Job, error) {
	if preset != "mp4-720p" && preset != "mp4-1080p" || !guestproto.ValidID(input) {
		return Job{}, ErrInvalid
	}
	if _, err := s.app(ctx, "files"); err != nil {
		return Job{}, err
	}
	var attemptID string
	err := s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		op, replay, err := operation(tx, device, key, "job", map[string]string{"inputId": input, "preset": preset, "retryGroup": retryGroup})
		if err != nil {
			return err
		}
		if replay {
			return tx.QueryRow("SELECT id FROM jobs WHERE operation_id=?", op.ID).Scan(&attemptID)
		}
		if err := state.RequireAdmission(tx); err != nil {
			return errors.Join(ErrConflict, err)
		}
		var size int64
		if err = tx.QueryRow("SELECT size FROM files WHERE id=? AND trash_until IS NULL", input).Scan(&size); err != nil {
			return ErrInvalid
		}
		if size > MaxFileBytes {
			return ErrInvalid
		}
		var active int
		if err = tx.QueryRow("SELECT count(*) FROM jobs WHERE state IN('queued','preparing','running','finalizing','cancelling')").Scan(&active); err != nil {
			return err
		}
		if active >= 8 {
			return ErrConflict
		}
		var used, reserved int64
		if err = tx.QueryRow("SELECT coalesce(sum(size),0)+(SELECT count(*) FROM orphan_objects WHERE workload='files')*? FROM files", MaxJobOutputBytes).Scan(&used); err != nil {
			return err
		}
		if err = tx.QueryRow("SELECT coalesce(sum(size),0) FROM transfers WHERE state IN('uploading','verifying','cancelling')").Scan(&reserved); err != nil {
			return err
		}
		if used+reserved+int64(active+1)*MaxJobOutputBytes > StorageQuota {
			return ErrConflict
		}
		group := retryGroup
		if group == "" {
			group = state.Random()
			if _, err = tx.Exec("INSERT INTO job_groups VALUES(?,?,?,?,?)", group, device, input, preset, time.Now().Unix()); err != nil {
				return err
			}
		} else {
			var originalInput, originalPreset string
			if err = tx.QueryRow("SELECT input_id,preset FROM job_groups WHERE id=?", group).Scan(&originalInput, &originalPreset); err != nil {
				return err
			}
			if originalInput != input || originalPreset != preset {
				return ErrConflict
			}
			var count int
			if err = tx.QueryRow("SELECT count(*) FROM jobs WHERE group_id=? AND state IN('queued','preparing','running','finalizing','cancelling')", group).Scan(&count); err != nil {
				return err
			}
			if count != 0 {
				return ErrConflict
			}
		}
		attemptID = state.Random()
		now := time.Now().Unix()
		_, err = tx.Exec("INSERT INTO jobs(id,device_id,input_id,preset,state,instance_id,operation_id,created_at,updated_at,group_id) VALUES(?,?,?,?,'queued',?,?,?,?,?)", attemptID, device, input, preset, state.Random(), op.ID, now, now, group)
		if err != nil {
			return err
		}
		return state.Event(tx, device, "job.queued", group, map[string]string{"attemptId": attemptID})
	})
	if err != nil {
		return Job{}, err
	}
	return s.attempt(ctx, attemptID)
}
func (s *Service) CancelJob(ctx context.Context, device, id, attemptID string) error {
	j, err := s.attempt(ctx, attemptID)
	if err != nil || j.ID != id {
		return ErrInvalid
	}
	if j.State == "succeeded" || j.State == "failed" || j.State == "cancelled" || j.State == "interrupted" {
		return nil
	}
	_, err = s.Store.DB.ExecContext(ctx, "UPDATE jobs SET state='cancelling',updated_at=? WHERE id=? AND state IN('queued','preparing','running','finalizing')", time.Now().Unix(), attemptID)
	if err != nil {
		return err
	}
	// The worker observes intent between chunks and polls. Stop enforcement does
	// not depend on the guest honoring its cancel request.
	if j.State != "queued" && s.Backend != nil {
		_, err = s.Backend.Apply(ctx, supervisor.Request{Version: 1, OperationID: state.Random(), Action: "stop", Revision: 2, InstanceID: j.InstanceID, PolicyGeneration: s.PolicyGeneration})
		if err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) jobPhase(ctx context.Context, j Job, phase string) error {
	expected := map[string]string{"preparing": "queued", "running": "preparing", "finalizing": "running"}[phase]
	if expected == "" {
		return ErrInvalid
	}
	return s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		result, err := tx.Exec("UPDATE jobs SET state=?,updated_at=? WHERE id=? AND state=?", phase, time.Now().Unix(), j.AttemptID, expected)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			var current string
			if err = tx.QueryRow("SELECT state FROM jobs WHERE id=?", j.AttemptID).Scan(&current); err != nil {
				return err
			}
			if current == "cancelling" || current == "cancelled" {
				return context.Canceled
			}
			return ErrConflict
		}
		var device string
		if err = tx.QueryRow("SELECT device_id FROM jobs WHERE id=?", j.AttemptID).Scan(&device); err != nil {
			return err
		}
		if phase == "preparing" {
			if _, err = tx.Exec("UPDATE operations SET state='executing',updated_at=? WHERE id=?", time.Now().Unix(), j.OperationID); err != nil {
				return err
			}
		}
		return state.Event(tx, device, "job."+phase, j.ID, map[string]string{"attemptId": j.AttemptID})
	})
}
func (s *Service) jobActive(ctx context.Context, j Job) error {
	var phase string
	err := s.Store.DB.QueryRowContext(ctx, "SELECT state FROM jobs WHERE id=?", j.AttemptID).Scan(&phase)
	if err != nil {
		return err
	}
	if phase == "cancelling" || phase == "cancelled" {
		return context.Canceled
	}
	return nil
}
func (s *Service) copyObject(ctx context.Context, j Job, source, destination, sourceID, destinationID string, size int64, expected string) error {
	hash := sha256.New()
	for offset := int64(0); offset < size; {
		if err := s.jobActive(ctx, j); err != nil {
			return err
		}
		chunk, err := s.call(ctx, source, guestproto.Request{Operation: "download", ObjectID: sourceID, Offset: offset})
		if err != nil {
			return err
		}
		if len(chunk.Data) == 0 || len(chunk.Data) > guestproto.ChunkSize || int64(len(chunk.Data)) > size-offset || sum(chunk.Data) != chunk.SHA256 {
			return ErrConflict
		}
		_, _ = hash.Write(chunk.Data)
		written, err := s.call(ctx, destination, guestproto.Request{Operation: "upload", ObjectID: destinationID, Offset: offset, Size: size, Data: chunk.Data, SHA256: chunk.SHA256})
		if err != nil {
			return err
		}
		offset += int64(len(chunk.Data))
		if written.Offset != offset {
			return ErrConflict
		}
	}
	if hex.EncodeToString(hash.Sum(nil)) != expected {
		return ErrConflict
	}
	finalized, err := s.call(ctx, destination, guestproto.Request{Operation: "finalize", ObjectID: destinationID, Size: size, SHA256: expected})
	if err != nil {
		return err
	}
	if finalized.Size != size || finalized.SHA256 != expected {
		return ErrConflict
	}
	return nil
}
func (s *Service) processJob(ctx context.Context) error {
	if s.Backend == nil {
		return nil
	}
	// Cancelled queue entries do not need a VM.
	rows, err := s.Store.DB.QueryContext(ctx, "SELECT "+jobColumns+" FROM jobs WHERE state='cancelling'")
	if err != nil {
		return err
	}
	cancelled := []Job{}
	for rows.Next() {
		j, e := scanJob(rows)
		if e != nil {
			rows.Close()
			return e
		}
		cancelled = append(cancelled, j)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, j := range cancelled {
		if j.StartRequested {
			if _, err = s.Backend.Apply(ctx, supervisor.Request{Version: 1, OperationID: state.Random(), Action: "stop", Revision: 2, InstanceID: j.InstanceID, PolicyGeneration: s.PolicyGeneration}); err != nil {
				continue
			}
		}
		if err = s.endJob(ctx, j, "cancelled", "CANCELLED", ""); err != nil {
			return err
		}
	}

	j, err := scanJob(s.Store.DB.QueryRowContext(ctx, "SELECT "+jobColumns+" FROM jobs WHERE state='queued' ORDER BY created_at,rowid LIMIT 1"))
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var authorized int
	if err = s.Store.DB.QueryRowContext(ctx, "SELECT count(*) FROM devices WHERE id=(SELECT device_id FROM jobs WHERE id=?) AND revoked_at IS NULL AND EXISTS(SELECT 1 FROM json_each(capabilities) WHERE value='jobs')", j.AttemptID).Scan(&authorized); err != nil {
		return err
	}
	if authorized != 1 {
		return s.endJob(ctx, j, "failed", "AUTHORIZATION_EXPIRED", "")
	}
	jobCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	if err = s.jobPhase(jobCtx, j, "preparing"); err != nil {
		return err
	}
	workErr := s.executeJob(jobCtx, j)
	cleanup, cancelCleanup := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelCleanup()
	var stopErr error
	current, readErr := s.attempt(cleanup, j.AttemptID)
	if readErr != nil {
		return readErr
	}
	if current.StartRequested {
		stopErr = s.releaseJobResources(cleanup, j)
	}
	latest, err := s.attempt(cleanup, j.AttemptID)
	if err != nil {
		return err
	}
	if latest.State == "succeeded" {
		if stopErr != nil {
			_, err = s.Store.DB.ExecContext(cleanup, "UPDATE jobs SET error_code='CLEANUP_PENDING' WHERE id=?", j.AttemptID)
			return errors.Join(stopErr, err)
		}
		return stopErr
	}
	phase, code := "failed", "WORKLOAD_FAILED"
	if errors.Is(workErr, context.Canceled) || latest.State == "cancelling" {
		phase, code = "cancelled", "CANCELLED"
	}
	if ctx.Err() != nil {
		phase, code = "interrupted", "CONTROLLER_STOPPED"
	}
	if stopErr != nil {
		phase, code = "interrupted", "STOP_REQUIRES_ATTENTION"
	}
	return s.endJob(cleanup, j, phase, code, "")
}
func (s *Service) executeJob(ctx context.Context, j Job) error {
	files, err := s.app(ctx, "files")
	if err != nil {
		return err
	}
	input, err := s.File(ctx, j.InputID)
	if err != nil || input.TrashUntil != nil {
		return ErrConflict
	}
	err = s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		result, err := tx.Exec("UPDATE jobs SET start_requested=1 WHERE id=? AND state='preparing'", j.AttemptID)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return context.Canceled
		}
		_, err = tx.Exec("INSERT OR IGNORE INTO settings(key,value) VALUES(?,json_object('state','pending','lastAttempt',0))", cleanupKey(j.AttemptID))
		return err
	})
	if err != nil {
		return err
	}
	instance, err := s.Backend.Apply(ctx, supervisor.Request{Version: 1, OperationID: j.OperationID, Action: "start", Revision: 1, InstanceID: j.InstanceID, Workload: "video", PolicyGeneration: s.PolicyGeneration})
	if err != nil {
		return err
	}
	if instance.State != "running" {
		return ErrUnavailable
	}
	// Boot readiness is an observation; it cannot authorize new resources.
	if err = s.waitGuest(ctx, j.InstanceID, j); err != nil {
		return err
	}
	if err = s.copyObject(ctx, j, files, j.InstanceID, j.InputID, j.InputID, input.Size, input.SHA256); err != nil {
		return err
	}
	if err = s.jobActive(ctx, j); err != nil {
		return err
	}
	if _, err = s.call(ctx, j.InstanceID, guestproto.Request{Operation: "run", ObjectID: j.AttemptID, InputID: j.InputID, Preset: j.Preset}); err != nil {
		return err
	}
	if err = s.jobPhase(ctx, j, "running"); err != nil {
		return err
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err = s.jobActive(ctx, j); err != nil {
				return err
			}
			result, e := s.call(ctx, j.InstanceID, guestproto.Request{Operation: "result", ObjectID: j.AttemptID})
			if e != nil {
				return e
			}
			switch result.State {
			case "running":
				continue
			case "succeeded":
				if result.Size <= 0 || result.Size > MaxJobOutputBytes || !hashPattern.MatchString(result.SHA256) {
					return ErrConflict
				}
				if err = s.jobPhase(ctx, j, "finalizing"); err != nil {
					return err
				}
				output := state.Random()
				unlock := s.lock(output)
				defer unlock()
				if _, err = s.Store.DB.ExecContext(ctx, "INSERT INTO orphan_objects VALUES(?,'files',?)", output, time.Now().Unix()); err != nil {
					return err
				}
				if err = s.copyObject(ctx, j, j.InstanceID, files, j.AttemptID, output, result.Size, result.SHA256); err != nil {
					return err
				}
				return s.Store.Transaction(ctx, func(tx *sql.Tx) error {
					var phase, device string
					if err := tx.QueryRow("SELECT state,device_id FROM jobs WHERE id=?", j.AttemptID).Scan(&phase, &device); err != nil {
						return err
					}
					if phase != "finalizing" {
						return context.Canceled
					}
					if _, err := tx.Exec("INSERT INTO files(id,name,size,sha256,created_at) VALUES(?,?,?,?,?)", output, "video-"+j.AttemptID[:12]+".mp4", result.Size, result.SHA256, time.Now().Unix()); err != nil {
						return err
					}
					if _, err := tx.Exec("UPDATE jobs SET state='succeeded',output_id=?,updated_at=? WHERE id=?", output, time.Now().Unix(), j.AttemptID); err != nil {
						return err
					}
					if _, err := tx.Exec("UPDATE operations SET state='succeeded',result=?,updated_at=? WHERE id=?", `{"outputId":"`+output+`"}`, time.Now().Unix(), j.OperationID); err != nil {
						return err
					}
					if _, err := tx.Exec("DELETE FROM orphan_objects WHERE id=?", output); err != nil {
						return err
					}
					return state.Event(tx, device, "job.succeeded", j.ID, map[string]string{"attemptId": j.AttemptID, "outputId": output})
				})
			default:
				return ErrConflict
			}
		}
	}
}
func (s *Service) waitGuest(ctx context.Context, id string, j Job) error {
	deadline := time.NewTimer(60 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if err := s.jobActive(ctx, j); err != nil {
			return err
		}
		if response, err := s.call(ctx, id, guestproto.Request{Operation: "health"}); err == nil && response.State == "ready" {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return ErrUnavailable
		case <-ticker.C:
		}
	}
}
func (s *Service) endJob(ctx context.Context, j Job, phase, code, output string) error {
	return s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		var device string
		if err := tx.QueryRow("SELECT device_id FROM jobs WHERE id=?", j.AttemptID).Scan(&device); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE jobs SET state=?,error_code=?,updated_at=? WHERE id=? AND state<>'succeeded'", phase, code, time.Now().Unix(), j.AttemptID); err != nil {
			return err
		}
		opPhase := "failed"
		if phase == "interrupted" {
			opPhase = "requires-action"
		}
		if _, err := tx.Exec("UPDATE operations SET state=?,result=?,updated_at=? WHERE id=? AND state<>'succeeded'", opPhase, `{"code":"`+code+`"}`, time.Now().Unix(), j.OperationID); err != nil {
			return err
		}
		return state.Event(tx, device, "job."+phase, j.ID, map[string]string{"attemptId": j.AttemptID, "code": code})
	})
}

func (s *Service) reconcileJobs(ctx context.Context) error {
	rows, err := s.Store.DB.QueryContext(ctx, "SELECT "+jobColumns+" FROM jobs WHERE state IN('preparing','running','finalizing','cancelling')")
	if err != nil {
		return err
	}
	items := []Job{}
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			rows.Close()
			return err
		}
		items = append(items, j)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, j := range items {
		if j.StartRequested {
			if s.Backend == nil {
				return ErrUnavailable
			}
			if _, err = s.Backend.Apply(ctx, supervisor.Request{Version: 1, OperationID: state.Random(), Action: "stop", Revision: 2, InstanceID: j.InstanceID, PolicyGeneration: s.PolicyGeneration}); err != nil {
				return err
			}
		}
		if err = s.endJob(ctx, j, "interrupted", "CONTROLLER_RESTARTED", ""); err != nil {
			return err
		}
	}
	return nil
}
