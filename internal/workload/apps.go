package workload

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/pranavreddyg17/home-node/internal/state"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

type App struct {
	Workload   string `json:"workload"`
	InstanceID string `json:"instanceId"`
	State      string `json:"state"`
	UpdatedAt  int64  `json:"updatedAt"`
}
type appIntent struct {
	Workload   string `json:"workload"`
	Action     string `json:"action"`
	InstanceID string `json:"instanceId"`
}

func (s *Service) Apps(ctx context.Context) ([]App, error) {
	apps := []App{}
	rows, err := s.Store.DB.QueryContext(ctx, "SELECT workload,instance_id,state,updated_at FROM apps ORDER BY workload")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var a App
		if err = rows.Scan(&a.Workload, &a.InstanceID, &a.State, &a.UpdatedAt); err != nil {
			return nil, err
		}
		apps = append(apps, a)
	}
	return apps, rows.Err()
}
func (s *Service) AppAction(ctx context.Context, device, key, name, action string) (Operation, error) {
	if name != "files" && name != "ai" || action != "start" && action != "stop" {
		return Operation{}, ErrInvalid
	}
	if s.Backend == nil {
		return Operation{}, ErrUnavailable
	}
	var op Operation
	err := s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		var phase, instance string
		err := tx.QueryRow("SELECT instance_id,state FROM apps WHERE workload=?", name).Scan(&instance, &phase)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if instance == "" {
			instance = state.Random()
		}
		// Hash the user's stable intent, not a new generated identifier, so retries
		// can recover the original operation after a network failure.
		var replay bool
		op, replay, err = operation(tx, device, key, "app."+action, map[string]string{"workload": name, "action": action})
		if err != nil || replay {
			return err
		}
		if phase == "starting" || phase == "stopping" || action == "start" && phase == "running" {
			return ErrConflict
		}
		intent, _ := json.Marshal(appIntent{Workload: name, Action: action, InstanceID: instance})
		if _, err = tx.Exec("UPDATE operations SET result=? WHERE id=?", string(intent), op.ID); err != nil {
			return err
		}
		op.Result = intent
		phase = "starting"
		if action == "stop" {
			phase = "stopping"
		}
		_, err = tx.Exec("INSERT INTO apps(workload,instance_id,state,operation_id,updated_at) VALUES(?,?,?,?,?) ON CONFLICT(workload) DO UPDATE SET state=excluded.state,operation_id=excluded.operation_id,updated_at=excluded.updated_at", name, instance, phase, op.ID, time.Now().Unix())
		return err
	})
	return op, err
}
func (s *Service) processApp(ctx context.Context) error {
	if s.Backend == nil {
		return nil
	}
	var opID, device, data, kind string
	var created int64
	err := s.Store.DB.QueryRowContext(ctx, "SELECT id,device_id,result,kind,created_at FROM operations WHERE state='pending' AND kind IN('app.start','app.stop') ORDER BY created_at LIMIT 1").Scan(&opID, &device, &data, &kind, &created)
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
	var authorized int
	err = s.Store.DB.QueryRowContext(ctx, "SELECT count(*) FROM devices WHERE id=? AND revoked_at IS NULL AND EXISTS(SELECT 1 FROM json_each(capabilities) WHERE value='admin')", device).Scan(&authorized)
	if err != nil {
		return err
	}
	if authorized != 1 || time.Now().Unix()-created > 300 {
		_, _ = s.Store.DB.ExecContext(ctx, "UPDATE apps SET state='failed' WHERE workload=?", intent.Workload)
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
	instance, err := s.Backend.Apply(ctx, supervisor.Request{Version: 1, OperationID: opID, InstanceID: intent.InstanceID, Workload: intent.Workload, Action: intent.Action, PolicyGeneration: s.Generation})
	if err != nil {
		_, _ = s.Store.DB.ExecContext(ctx, "UPDATE apps SET state='failed',updated_at=? WHERE workload=?", time.Now().Unix(), intent.Workload)
		return s.finish(ctx, opID, "failed", map[string]string{"code": "RUNTIME_BLOCKED"})
	}
	if _, err = s.Store.DB.ExecContext(ctx, "UPDATE apps SET state=?,updated_at=? WHERE workload=?", instance.State, time.Now().Unix(), intent.Workload); err != nil {
		return err
	}
	return s.finish(ctx, opID, "succeeded", instance)
}
func (s *Service) Reconcile(ctx context.Context) error {
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
			observed, err := s.Backend.Apply(ctx, supervisor.Request{Version: 1, OperationID: state.Random(), Action: "inspect", InstanceID: a.InstanceID, PolicyGeneration: s.Generation})
			if err == nil {
				phase = observed.State
			}
		}
		if _, err = s.Store.DB.ExecContext(ctx, "UPDATE apps SET state=?,updated_at=? WHERE workload=?", phase, time.Now().Unix(), a.Workload); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = s.processApp(ctx)
		}
	}
}
