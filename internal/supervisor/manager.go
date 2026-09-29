// Package supervisor is the independently authorized runtime boundary. Its
// database, policy, images and channels must be inaccessible to the API UID.
package supervisor

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/pranavreddyg17/home-node/internal/catalog"
	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"github.com/pranavreddyg17/home-node/internal/state"
)

var ErrPolicy = errors.New("supervisor policy denied")
var ErrCapacity = errors.New("workload capacity unavailable")

type Policy struct {
	Generation       int64  `json:"generation"`
	MemoryMiB        int    `json:"memoryMiB"`
	VCPUs            int    `json:"vcpus"`
	MaxInstances     int    `json:"maxInstances"`
	DiskReserveBytes int64  `json:"diskReserveBytes"`
	ControllerUID    uint32 `json:"controllerUid"`
	TransferUID      uint32 `json:"transferUid"`
}

func (p Policy) Validate() error {
	if p.Generation < 1 || p.MemoryMiB < 1024 || p.MemoryMiB > 131072 || p.VCPUs < 1 || p.VCPUs > 64 || p.MaxInstances < 1 || p.MaxInstances > 4 || p.DiskReserveBytes < 4*catalog.GiB || p.ControllerUID == 0 || p.TransferUID == 0 || p.ControllerUID == p.TransferUID {
		return ErrPolicy
	}
	return nil
}

type Request struct {
	Version          int    `json:"version"`
	OperationID      string `json:"operationId"`
	Action           string `json:"action"`
	Workload         string `json:"workload,omitempty"`
	InstanceID       string `json:"instanceId"`
	PolicyGeneration int64  `json:"policyGeneration"`
}
type Instance struct {
	CreatedAt   int64  `json:"createdAt"`
	ID          string `json:"id"`
	Workload    string `json:"workload"`
	State       string `json:"state"`
	Desired     string `json:"desired"`
	ImageSHA256 string `json:"imageSha256"`
	MemoryMiB   int    `json:"memoryMiB"`
	VCPUs       int    `json:"vcpus"`
	DataBytes   int64  `json:"dataBytes"`
}
type Backend interface {
	ValidateHost(context.Context, Policy) error
	Prepare(context.Context, Domain) error
	Start(context.Context, Domain) error
	Stop(context.Context, string) error
	Running(context.Context, string) (bool, error)
	Verify(context.Context, Domain) error
	FreeBytes(string) (int64, error)
}
type Manager struct {
	Store                     *state.Store
	Policy                    Policy
	Manifest                  catalog.Manifest
	Images, Volumes, Channels string
	Backend                   Backend
	startMu                   sync.Mutex
}

func (m *Manager) Initialize(ctx context.Context) error {
	if err := m.Policy.Validate(); err != nil {
		return err
	}
	for _, dir := range []string{m.Images, m.Volumes, m.Channels} {
		if !filepath.IsAbs(dir) {
			return ErrPolicy
		}
	}
	_, err := m.Store.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS runtime_instances(id TEXT PRIMARY KEY, workload TEXT NOT NULL, state TEXT NOT NULL, desired TEXT NOT NULL, image_sha256 TEXT NOT NULL, memory_mib INTEGER NOT NULL, vcpus INTEGER NOT NULL, data_bytes INTEGER NOT NULL, created_at INTEGER NOT NULL); CREATE TABLE IF NOT EXISTS runtime_operations(id TEXT PRIMARY KEY, request_hash TEXT NOT NULL, instance_id TEXT NOT NULL, state TEXT NOT NULL);`)
	if err != nil {
		return err
	}
	return m.Store.Transaction(ctx, func(tx *sql.Tx) error {
		var previous string
		err := tx.QueryRow("SELECT value FROM settings WHERE key='policy-generation'").Scan(&previous)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		data, _ := json.Marshal(m.Policy)
		hash := state.Hash(string(data))
		if err == nil {
			version, e := strconv.ParseInt(previous, 10, 64)
			if e != nil {
				return e
			}
			if version > m.Policy.Generation {
				return ErrPolicy
			}
			if version == m.Policy.Generation {
				var stored string
				if e = tx.QueryRow("SELECT value FROM settings WHERE key='policy-hash'").Scan(&stored); e != nil {
					return e
				}
				if stored != hash {
					return ErrPolicy
				}
			}
		}
		if _, err = tx.Exec("INSERT INTO settings VALUES('policy-generation',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", strconv.FormatInt(m.Policy.Generation, 10)); err != nil {
			return err
		}
		_, err = tx.Exec("INSERT INTO settings VALUES('policy-hash',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", hash)
		return err
	})
}
func (m *Manager) Inspect(ctx context.Context, id string) (Instance, error) {
	var i Instance
	err := m.Store.DB.QueryRowContext(ctx, "SELECT id,workload,state,desired,image_sha256,memory_mib,vcpus,data_bytes,created_at FROM runtime_instances WHERE id=?", id).Scan(&i.ID, &i.Workload, &i.State, &i.Desired, &i.ImageSHA256, &i.MemoryMiB, &i.VCPUs, &i.DataBytes, &i.CreatedAt)
	return i, err
}
func (m *Manager) Apply(ctx context.Context, r Request) (Instance, error) {
	if r.Version != 1 || !guestproto.ValidID(r.OperationID) || !guestproto.ValidID(r.InstanceID) || r.PolicyGeneration != m.Policy.Generation {
		return Instance{}, ErrPolicy
	}
	switch r.Action {
	case "start":
		return m.start(ctx, r)
	case "stop":
		return m.stop(ctx, r)
	case "inspect":
		return m.Inspect(ctx, r.InstanceID)
	default:
		return Instance{}, ErrPolicy
	}
}
func (m *Manager) operation(tx *sql.Tx, r Request) (bool, error) {
	data, _ := json.Marshal(r)
	hash := state.Hash(string(data))
	var existing, phase string
	err := tx.QueryRow("SELECT request_hash,state FROM runtime_operations WHERE id=?", r.OperationID).Scan(&existing, &phase)
	if err == nil {
		if existing != hash {
			return false, ErrPolicy
		}
		return true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	_, err = tx.Exec("INSERT INTO runtime_operations(id,request_hash,instance_id,state) VALUES(?,?,?,'pending')", r.OperationID, hash, r.InstanceID)
	return false, err
}
func (m *Manager) start(ctx context.Context, r Request) (Instance, error) {
	m.startMu.Lock()
	defer m.startMu.Unlock()
	if !m.Manifest.Expires.After(time.Now()) {
		return Instance{}, catalog.ErrUntrusted
	}
	image, err := m.Manifest.Image(r.Workload)
	if err != nil {
		return Instance{}, err
	}
	if err = m.Backend.ValidateHost(ctx, m.Policy); err != nil {
		return Instance{}, err
	}
	var replay bool
	err = m.Store.Transaction(ctx, func(tx *sql.Tx) error {
		var e error
		replay, e = m.operation(tx, r)
		if e != nil || replay {
			return e
		}
		var existingWorkload, existingDigest, existingState string
		existingErr := tx.QueryRow("SELECT workload,image_sha256,state FROM runtime_instances WHERE id=?", r.InstanceID).Scan(&existingWorkload, &existingDigest, &existingState)
		restarting := existingErr == nil
		if existingErr != nil && !errors.Is(existingErr, sql.ErrNoRows) {
			return existingErr
		}
		if restarting && (r.Workload == "video" || existingWorkload != r.Workload || existingDigest != image.SHA256 || existingState == "running" || existingState == "preparing" || existingState == "stopping") {
			return ErrPolicy
		}
		var count, memory, cpus int
		if e = tx.QueryRow("SELECT count(*),coalesce(sum(memory_mib+512),0),coalesce(sum(vcpus),0) FROM runtime_instances WHERE state IN('preparing','running','stopping')").Scan(&count, &memory, &cpus); e != nil {
			return e
		}
		if count >= m.Policy.MaxInstances || memory+image.MemoryMiB+512 > m.Policy.MemoryMiB || cpus+image.VCPUs > m.Policy.VCPUs {
			return ErrCapacity
		}
		free, e := m.Backend.FreeBytes(m.Volumes)
		if e != nil {
			return e
		}
		required := m.Policy.DiskReserveBytes
		if !restarting {
			required += image.DataBytes
		}
		if free < required {
			return ErrCapacity
		}
		var same int
		if e = tx.QueryRow("SELECT count(*) FROM runtime_instances WHERE workload=? AND desired='running'", r.Workload).Scan(&same); e != nil {
			return e
		}
		if r.Workload != "video" && same > 0 {
			return ErrCapacity
		}
		if restarting {
			_, e = tx.Exec("UPDATE runtime_instances SET state='preparing',desired='running',created_at=? WHERE id=?", time.Now().Unix(), r.InstanceID)
		} else {
			_, e = tx.Exec("INSERT INTO runtime_instances VALUES(?,?,'preparing','running',?,?,?,?,?)", r.InstanceID, r.Workload, image.SHA256, image.MemoryMiB, image.VCPUs, image.DataBytes, time.Now().Unix())
		}
		return e
	})
	if err != nil {
		return Instance{}, err
	}
	if replay {
		return m.Inspect(ctx, r.InstanceID)
	}
	fail := func(cause error) (Instance, error) {
		stopCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		stopErr := m.Backend.Stop(stopCtx, r.InstanceID)
		phase := "failed"
		if stopErr != nil {
			phase = "stopping"
		}
		_, _ = m.Store.DB.Exec("UPDATE runtime_instances SET state=?,desired='stopped' WHERE id=?", phase, r.InstanceID)
		_, _ = m.Store.DB.Exec("UPDATE runtime_operations SET state='failed' WHERE id=?", r.OperationID)
		return Instance{}, cause
	}
	systemPath, err := catalog.VerifyImage(m.Images, image)
	if err != nil {
		return fail(err)
	}
	domain := Domain{ID: r.InstanceID, Image: image, SystemPath: systemPath, DataPath: filepath.Join(m.Volumes, r.InstanceID+".raw"), ChannelPath: filepath.Join(m.Channels, r.InstanceID, "adapter.sock")}
	if err = m.Backend.Prepare(ctx, domain); err != nil {
		return fail(err)
	}
	current, err := m.Inspect(ctx, r.InstanceID)
	if err != nil {
		return fail(err)
	}
	if current.Desired != "running" {
		return fail(errors.New("start cancelled"))
	}
	if err = m.Backend.Start(ctx, domain); err != nil {
		return fail(err)
	}
	if err = m.Backend.Verify(ctx, domain); err != nil {
		return fail(err)
	}
	current, err = m.Inspect(ctx, r.InstanceID)
	if err != nil {
		return fail(err)
	}
	if current.Desired != "running" {
		return fail(errors.New("start cancelled"))
	}
	err = m.Store.Transaction(ctx, func(tx *sql.Tx) error {
		result, e := tx.Exec("UPDATE runtime_instances SET state='running' WHERE id=? AND desired='running'", r.InstanceID)
		if e != nil {
			return e
		}
		n, e := result.RowsAffected()
		if e != nil {
			return e
		}
		if n != 1 {
			return errors.New("start cancelled")
		}
		_, e = tx.Exec("UPDATE runtime_operations SET state='succeeded' WHERE id=?", r.OperationID)
		return e
	})
	if err != nil {
		return fail(err)
	}
	return m.Inspect(ctx, r.InstanceID)
}
func (m *Manager) stop(ctx context.Context, r Request) (Instance, error) {
	err := m.Store.Transaction(ctx, func(tx *sql.Tx) error {
		_, err := m.operation(tx, r)
		if err != nil {
			return err
		}
		result, err := tx.Exec("UPDATE runtime_instances SET desired='stopped',state='stopping' WHERE id=?", r.InstanceID)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrPolicy
		}
		return nil
	})
	if err != nil {
		return Instance{}, err
	}
	if err = m.Backend.Stop(ctx, r.InstanceID); err != nil {
		return Instance{}, err
	}
	err = m.Store.Transaction(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE runtime_instances SET state='stopped' WHERE id=?", r.InstanceID); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE runtime_operations SET state='succeeded' WHERE id=?", r.OperationID)
		return err
	})
	if err != nil {
		return Instance{}, err
	}
	return m.Inspect(ctx, r.InstanceID)
}

// Reconcile never repeats an uncertain job. At startup it stops each recorded
// VM before marking interrupted operations; apps are restarted by new intent.
func (m *Manager) Reconcile(ctx context.Context) error {
	rows, err := m.Store.DB.QueryContext(ctx, "SELECT id FROM runtime_instances WHERE state IN('preparing','running','stopping')")
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
		if !guestproto.ValidID(id) {
			return ErrPolicy
		}
		if err = m.Backend.Stop(ctx, id); err != nil {
			return fmt.Errorf("cannot stop uncertain instance: %w", err)
		}
		if _, err = m.Store.DB.ExecContext(ctx, "UPDATE runtime_instances SET state='interrupted',desired='stopped' WHERE id=?", id); err != nil {
			return err
		}
	}
	_, err = m.Store.DB.ExecContext(ctx, "UPDATE runtime_operations SET state='interrupted' WHERE state='pending'")
	return err
}
func (m *Manager) Channel(ctx context.Context, id string) (string, error) {
	if !guestproto.ValidID(id) {
		return "", ErrPolicy
	}
	instance, err := m.Inspect(ctx, id)
	if err != nil || instance.State != "running" {
		return "", ErrPolicy
	}
	path := filepath.Join(m.Channels, id, "adapter.sock")
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSocket == 0 {
		return "", ErrPolicy
	}
	return path, nil
}

// Audit enforces expiration, the finite-job wall clock and confinement even
// when the controller or guest adapter is unavailable or dishonest.
func (m *Manager) Audit(ctx context.Context) error {
	rows, err := m.Store.DB.QueryContext(ctx, "SELECT id FROM runtime_instances WHERE state IN('running','stopping')")
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
	hostErr := m.Backend.ValidateHost(ctx, m.Policy)
	for _, id := range ids {
		i, err := m.Inspect(ctx, id)
		if err != nil {
			return err
		}
		image, imageErr := m.Manifest.Image(i.Workload)
		stop := hostErr != nil || imageErr != nil || !m.Manifest.Expires.After(time.Now()) || i.State == "stopping" || i.Workload == "video" && time.Now().Unix()-i.CreatedAt >= 1800
		if !stop {
			d := Domain{ID: id, Image: image, SystemPath: filepath.Join(m.Images, image.SHA256+".raw"), DataPath: filepath.Join(m.Volumes, id+".raw"), ChannelPath: filepath.Join(m.Channels, id, "adapter.sock")}
			stop = m.Backend.Verify(ctx, d) != nil
		}
		if stop {
			if _, err = m.Store.DB.ExecContext(ctx, "UPDATE runtime_instances SET state='stopping',desired='stopped' WHERE id=?", id); err != nil {
				return err
			}
			if err = m.Backend.Stop(ctx, id); err != nil {
				return err
			}
			if _, err = m.Store.DB.ExecContext(ctx, "UPDATE runtime_instances SET state='interrupted' WHERE id=?", id); err != nil {
				return err
			}
		}
	}
	return nil
}
