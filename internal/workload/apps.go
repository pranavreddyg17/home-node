package workload

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"github.com/pranavreddyg17/home-node/internal/state"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

type App struct {
	Revision   int64  `json:"revision"`
	Workload   string `json:"workload"`
	InstanceID string `json:"instanceId"`
	State      string `json:"state"`
	UpdatedAt  int64  `json:"updatedAt"`
}
type appIntent struct {
	CooperativeStop bool   `json:"cooperativeStop,omitempty"`
	Revision        int64  `json:"revision"`
	Workload        string `json:"workload"`
	Action          string `json:"action"`
	InstanceID      string `json:"instanceId"`
}

func (s *Service) Apps(ctx context.Context) ([]App, error) {
	apps := []App{}
	rows, err := s.Store.DB.QueryContext(ctx, "SELECT workload,instance_id,state,updated_at,revision FROM apps ORDER BY workload")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var a App
		if err = rows.Scan(&a.Workload, &a.InstanceID, &a.State, &a.UpdatedAt, &a.Revision); err != nil {
			return nil, err
		}
		apps = append(apps, a)
	}
	return apps, rows.Err()
}

func (s *Service) AppAction(ctx context.Context, device, key, name, action string) (Operation, error) {
	var op Operation
	err := s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		var err error
		op, err = s.AppActionInTransaction(tx, device, key, name, action)
		return err
	})
	return op, err
}

// AppActionInTransaction persists intent only. Approval consumption can share
// this transaction; runtime effects happen later through the operation worker.
func (s *Service) AppActionInTransaction(tx *sql.Tx, device, key, name, action string) (Operation, error) {
	if name != "files" && name != "ai" || action != "start" && action != "stop" {
		return Operation{}, ErrInvalid
	}
	if s.Backend == nil {
		return Operation{}, ErrUnavailable
	}
	var op Operation
	var phase, instance string
	var revision int64
	err := tx.QueryRow("SELECT instance_id,state,revision FROM apps WHERE workload=?", name).Scan(&instance, &phase, &revision)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return op, err
	}
	if instance == "" {
		instance = state.Random()
	}
	// Hash the user's stable intent, not a new generated identifier, so retries
	// can recover the original operation after a network failure.
	var replay bool
	op, replay, err = operation(tx, device, key, "app."+action, map[string]string{"workload": name, "action": action})
	if err != nil || replay {
		return op, err
	}
	if err := state.RequireAdmission(tx); err != nil {
		return Operation{}, errors.Join(ErrConflict, err)
	}
	if phase == "stopping" || action == "start" && (phase == "running" || phase == "starting") {
		return Operation{}, ErrConflict
	}
	intent, _ := json.Marshal(appIntent{Workload: name, Action: action, InstanceID: instance, Revision: revision + 1, CooperativeStop: action == "stop" && phase == "running"})
	if _, err = tx.Exec("UPDATE operations SET result=? WHERE id=?", string(intent), op.ID); err != nil {
		return op, err
	}
	op.Result = intent
	phase = "starting"
	if action == "stop" {
		phase = "stopping"
	}
	_, err = tx.Exec("INSERT INTO apps(workload,instance_id,state,operation_id,updated_at,revision) VALUES(?,?,?,?,?,?) ON CONFLICT(workload) DO UPDATE SET state=excluded.state,operation_id=excluded.operation_id,updated_at=excluded.updated_at,revision=excluded.revision", name, instance, phase, op.ID, time.Now().Unix(), revision+1)
	return op, err
}

// AppApprovalResources identifies the logical app and its exact current instance
// and revision. A never-created app has revision zero and no instance ID.
func (s *Service) AppApprovalResources(tx *sql.Tx, name string) ([]string, error) {
	if name != "files" && name != "ai" {
		return nil, ErrInvalid
	}
	var instance string
	var revision int64
	err := tx.QueryRow("SELECT instance_id,revision FROM apps WHERE workload=?", name).Scan(&instance, &revision)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if revision < 0 {
		return nil, ErrConflict
	}
	resources := []string{name, "revision_" + strconv.FormatInt(revision, 10)}
	if instance != "" {
		resources = append(resources, instance)
	}
	return resources, nil
}

// AppActionAtResourcesInTransaction refuses an app changed since approval.
func (s *Service) AppActionAtResourcesInTransaction(tx *sql.Tx, device, key, name, action string, expected []string) (Operation, error) {
	current, err := s.AppApprovalResources(tx, name)
	if err != nil {
		return Operation{}, err
	}
	if !slices.Equal(current, expected) {
		return Operation{}, ErrConflict
	}
	return s.AppActionInTransaction(tx, device, key, name, action)
}

func (s *Service) processApp(ctx context.Context) error { return s.processAppKind(ctx, "") }
func (s *Service) processAppKind(ctx context.Context, selectedKind string) error {
	if s.Backend == nil {
		return nil
	}
	var opID, device, data, kind string
	var created int64
	err := s.Store.DB.QueryRowContext(ctx, "SELECT id,device_id,result,kind,created_at FROM operations WHERE state='pending' AND kind IN('app.start','app.stop') AND (?='' OR kind=?) ORDER BY created_at LIMIT 1", selectedKind, selectedKind).Scan(&opID, &device, &data, &kind, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var intent appIntent
	if err = json.Unmarshal([]byte(data), &intent); err != nil {
		return err
	}
	var currentOperation string
	if err = s.Store.DB.QueryRowContext(ctx, "SELECT operation_id FROM apps WHERE workload=?", intent.Workload).Scan(&currentOperation); err != nil {
		return err
	}
	if currentOperation != opID {
		return s.finish(ctx, opID, "failed", map[string]string{"code": "SUPERSEDED"})
	}
	var authorized int
	err = s.Store.DB.QueryRowContext(ctx, "SELECT count(*) FROM devices WHERE id=? AND revoked_at IS NULL AND EXISTS(SELECT 1 FROM json_each(capabilities) WHERE value='admin')", device).Scan(&authorized)
	if err != nil {
		return err
	}
	if authorized != 1 || time.Now().Unix()-created > 300 {
		_, _ = s.Store.DB.ExecContext(ctx, "UPDATE apps SET state='failed' WHERE workload=? AND operation_id=?", intent.Workload, opID)
		return s.finish(ctx, opID, "failed", map[string]string{"code": "AUTHORIZATION_EXPIRED"})
	}
	result, err := s.Store.DB.ExecContext(ctx, "UPDATE operations SET state='executing',updated_at=? WHERE id=? AND state='pending'", time.Now().Unix(), opID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil || n != 1 {
		return err
	}
	runtimeAction := intent.Action
	if intent.CooperativeStop {
		if intent.Action != "stop" {
			return ErrConflict
		}
		runtimeAction = "shutdown"
	}
	instance, err := s.Backend.Apply(ctx, supervisor.Request{Version: 1, OperationID: opID, InstanceID: intent.InstanceID, Workload: intent.Workload, Action: runtimeAction, Revision: intent.Revision, PolicyGeneration: s.PolicyGeneration})
	if err == nil && runtimeAction == "shutdown" && instance.State != "stopped" {
		err = ErrConflict
	}
	if err == nil && intent.Action == "start" && instance.State == "running" {
		err = s.waitApp(ctx, intent.Workload, intent.InstanceID, opID)
		if err != nil {
			cleanup, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			_, _ = s.Backend.Apply(cleanup, supervisor.Request{Version: 1, OperationID: state.Random(), Action: "stop", InstanceID: intent.InstanceID, Revision: intent.Revision, PolicyGeneration: s.PolicyGeneration})
			cancel()
		}
	}
	if err != nil {
		_, _ = s.Store.DB.ExecContext(ctx, "UPDATE apps SET state='failed',updated_at=? WHERE workload=? AND operation_id=?", time.Now().Unix(), intent.Workload, opID)
		return s.finish(ctx, opID, "failed", map[string]string{"code": "RUNTIME_BLOCKED"})
	}
	if _, err = s.Store.DB.ExecContext(ctx, "UPDATE apps SET state=?,updated_at=? WHERE workload=? AND operation_id=?", instance.State, time.Now().Unix(), intent.Workload, opID); err != nil {
		return err
	}
	return s.finish(ctx, opID, "succeeded", instance)
}
func (s *Service) Reconcile(ctx context.Context) error {
	if _, err := s.Store.DB.ExecContext(ctx, "INSERT OR IGNORE INTO settings(key,value) SELECT 'job.cleanup.'||id,json_object('state','pending','lastAttempt',0) FROM jobs WHERE start_requested=1"); err != nil {
		return err
	}
	if err := s.reconcileJobs(ctx); err != nil {
		return err
	}
	if _, err := s.Store.DB.ExecContext(ctx, "UPDATE operations SET state='requires-action' WHERE state='executing'"); err != nil {
		return err
	}
	apps, err := s.Apps(ctx)
	if err != nil {
		return err
	}
	for _, a := range apps {
		phase := "failed"
		if s.Backend != nil {
			observed, err := s.Backend.Apply(ctx, supervisor.Request{Version: 1, OperationID: state.Random(), Action: "inspect", InstanceID: a.InstanceID, PolicyGeneration: s.PolicyGeneration})
			if err == nil {
				phase = observed.State
				a.Revision = max(a.Revision, observed.Revision)
			}
		}
		if _, err = s.Store.DB.ExecContext(ctx, "UPDATE apps SET state=?,updated_at=?,revision=? WHERE workload=?", phase, time.Now().Unix(), a.Revision, a.Workload); err != nil {
			return err
		}
	}
	return s.reconcileGenerations(ctx)
}
func (s *Service) Run(ctx context.Context) {
	var workers sync.WaitGroup
	for _, work := range []func(context.Context) error{func(ctx context.Context) error { return s.processAppKind(ctx, "app.start") }, func(ctx context.Context) error { return s.processAppKind(ctx, "app.stop") }, s.processJob, s.processGeneration, s.expireTransfer, s.expireTrash, s.cleanupJobResources, s.cleanupOrphanObject, s.processConversationDeletion} {
		workers.Add(1)
		go func(fn func(context.Context) error) {
			defer workers.Done()
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if err := fn(ctx); err != nil && ctx.Err() == nil {
						slog.Error("workload operation requires attention")
					}
				}
			}
		}(work)
	}
	workers.Wait()
}

func (s *Service) waitApp(ctx context.Context, workload, instance, operation string) error {
	deadline, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		var current string
		if err := s.Store.DB.QueryRowContext(deadline, "SELECT operation_id FROM apps WHERE workload=?", workload).Scan(&current); err != nil {
			return err
		}
		if current != operation {
			return ErrConflict
		}
		if response, err := s.call(deadline, instance, guestproto.Request{Operation: "health"}); err == nil && response.State == "ready" {
			return nil
		}
		select {
		case <-deadline.Done():
			return deadline.Err()
		case <-ticker.C:
		}
	}
}
