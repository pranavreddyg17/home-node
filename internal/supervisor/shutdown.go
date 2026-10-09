package supervisor

import (
	"context"
	"database/sql"
	"strconv"
	"time"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
)

// Shutdown requests cooperative ACPI poweroff and observes domain exit. It never
// falls back to destroy. Domain exit alone does not attest a clean guest unmount;
// backup orchestration must establish that separately before copying disks.
// The supervisor journals ownership before invoking this primitive.
func (b LinuxBackend) Shutdown(ctx context.Context, id string) error {
	deadline, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	return awaitShutdown(deadline, id, b.Running, func(ctx context.Context, id string) error {
		_, err := command(ctx, "", "/usr/bin/virsh", "--connect", "qemu:///system", "shutdown", "homenode-"+id, "--mode", "acpi")
		return err
	}, 250*time.Millisecond)
}

func awaitShutdown(ctx context.Context, id string, running func(context.Context, string) (bool, error), request func(context.Context, string) error, interval time.Duration) error {
	if !guestproto.ValidID(id) || interval <= 0 {
		return ErrPolicy
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	active, err := running(ctx, id)
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if !active {
		return nil
	}
	if err = request(ctx, id); err != nil {
		return err
	}
	for {
		if err = ctx.Err(); err != nil {
			return err
		}
		active, err = running(ctx, id)
		if err != nil {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		if !active {
			return nil
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func shutdownDeadlineKey(id string) string { return "runtime.shutdown-deadline." + id }

type cooperativeShutdownBackend interface {
	Shutdown(context.Context, string) error
}

func shutdownOwnerKey(id string) string          { return "runtime.shutdown-owner." + id }
func shutdownSocketRevisionKey(id string) string { return "runtime.shutdown-socket-revision." + id }

func (m *Manager) shutdown(ctx context.Context, r Request) (Instance, error) {
	backend, ok := m.Backend.(cooperativeShutdownBackend)
	if !ok {
		return Instance{}, ErrPolicy
	}
	var completed bool
	var deadline int64
	err := m.Store.Transaction(ctx, func(tx *sql.Tx) error {
		replay, err := m.operation(tx, r)
		if err != nil {
			return err
		}
		var workload, phase string
		var revision int64
		if err = tx.QueryRow("SELECT workload,state,revision FROM runtime_instances WHERE id=?", r.InstanceID).Scan(&workload, &phase, &revision); err != nil {
			return err
		}
		if workload != "files" && workload != "ai" || r.Workload != "" && r.Workload != workload {
			return ErrPolicy
		}
		if replay {
			var operationPhase string
			if err = tx.QueryRow("SELECT state FROM runtime_operations WHERE id=?", r.OperationID).Scan(&operationPhase); err != nil {
				return err
			}
			if operationPhase != "succeeded" || phase != "stopped" || revision != r.Revision {
				return ErrPolicy
			}
			completed = true
			return nil
		}
		if phase != "running" || r.Revision <= revision {
			return ErrPolicy
		}
		if m.GuestUIDPool != nil {
			switch m.Backend.(type) {
			case LinuxBackend, *LinuxBackend:
				if err := m.bindShutdownSocketRevision(ctx, tx, r.InstanceID, revision); err != nil {
					return err
				}
			}
		}
		deadline = time.Now().Unix() + 90
		if _, err = tx.Exec("INSERT INTO runtime_stops VALUES(?,?) ON CONFLICT(instance_id) DO UPDATE SET revision=max(revision,excluded.revision)", r.InstanceID, r.Revision); err != nil {
			return err
		}
		if _, err = tx.Exec("UPDATE runtime_instances SET state='shutting-down',desired='stopped',revision=? WHERE id=?", r.Revision, r.InstanceID); err != nil {
			return err
		}
		for key, value := range map[string]string{shutdownDeadlineKey(r.InstanceID): strconv.FormatInt(deadline, 10), shutdownOwnerKey(r.InstanceID): r.OperationID} {
			if _, err = tx.Exec("INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", key, value); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Instance{}, err
	}
	if completed {
		return m.Inspect(ctx, r.InstanceID)
	}
	shutdownCtx, cancel := context.WithDeadline(ctx, time.Unix(deadline, 0))
	defer cancel()
	if err = backend.Shutdown(shutdownCtx, r.InstanceID); err != nil {
		return Instance{}, err
	}
	running, err := m.Backend.Running(shutdownCtx, r.InstanceID)
	if err != nil {
		return Instance{}, err
	}
	if running {
		return Instance{}, ErrPolicy
	}
	err = m.Store.Transaction(shutdownCtx, func(tx *sql.Tx) error {
		result, err := tx.Exec(`UPDATE runtime_instances SET state='stopped' WHERE id=? AND state='shutting-down' AND revision=?
   AND EXISTS(SELECT 1 FROM settings WHERE key=? AND value=?)
   AND EXISTS(SELECT 1 FROM settings WHERE key=? AND value=? AND CAST(value AS INTEGER)>unixepoch())`, r.InstanceID, r.Revision, shutdownOwnerKey(r.InstanceID), r.OperationID, shutdownDeadlineKey(r.InstanceID), strconv.FormatInt(deadline, 10))
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return ErrPolicy
		}
		result, err = tx.Exec("UPDATE runtime_operations SET state='succeeded' WHERE id=? AND state='pending'", r.OperationID)
		if err != nil {
			return err
		}
		count, err = result.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return ErrPolicy
		}
		_, err = tx.Exec("DELETE FROM settings WHERE key IN(?,?,?)", shutdownOwnerKey(r.InstanceID), shutdownDeadlineKey(r.InstanceID), shutdownSocketRevisionKey(r.InstanceID))
		return err
	})
	if err != nil {
		return Instance{}, err
	}
	return m.Inspect(ctx, r.InstanceID)
}

// bindShutdownSocketRevision runs in the same transaction as the stop request.
// It requires existing launch provenance; it cannot create a socket receipt.
func (m *Manager) bindShutdownSocketRevision(ctx context.Context, tx *sql.Tx, id string, revision int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if m == nil || tx == nil || revision < 1 {
		return ErrPolicy
	}
	if err := requireRuntimeAdmission(tx); err != nil {
		return err
	}
	if err := m.validateChannelOwnershipInventory(ctx, tx); err != nil {
		return err
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM runtime_channel_sockets s JOIN runtime_instances r ON r.id=s.instance_id WHERE s.instance_id=? AND s.revision=? AND s.retired=0 AND s.retirement_started=0 AND r.revision=s.revision AND r.state='running' AND r.desired='running'`, id, revision).Scan(&count); err != nil {
		return err
	}
	if count != 1 {
		return ErrPolicy
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, shutdownSocketRevisionKey(id), strconv.FormatInt(revision, 10))
	return err
}
