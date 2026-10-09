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
	Revision         int64  `json:"revision"`
	Version          int    `json:"version"`
	OperationID      string `json:"operationId"`
	Action           string `json:"action"`
	Workload         string `json:"workload,omitempty"`
	InstanceID       string `json:"instanceId"`
	PolicyGeneration int64  `json:"policyGeneration"`
}
type Instance struct {
	GuestUID    uint32 `json:"guestUid,omitempty"`
	Revision    int64  `json:"revision"`
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
	CleanupPreparation(context.Context, string, string, int64) error
}
type Manager struct {
	GuestUIDPool              *GuestUIDPool // independently qualified, exclusively provisioned host policy
	GuestGID                  uint32
	Store                     *state.Store
	Policy                    Policy
	Manifest                  catalog.Manifest
	Images, Volumes, Channels string
	Backend                   Backend
	startMu                   sync.Mutex
	runtimeMu                 sync.RWMutex // external calls versus maintenance acquisition
	shuttingDown              bool         // guarded by startMu
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
	_, err := m.Store.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS runtime_instances(id TEXT PRIMARY KEY, workload TEXT NOT NULL, state TEXT NOT NULL, desired TEXT NOT NULL, image_sha256 TEXT NOT NULL, memory_mib INTEGER NOT NULL, vcpus INTEGER NOT NULL, data_bytes INTEGER NOT NULL, created_at INTEGER NOT NULL); CREATE TABLE IF NOT EXISTS runtime_stops(instance_id TEXT PRIMARY KEY); CREATE TABLE IF NOT EXISTS runtime_operations(id TEXT PRIMARY KEY, request_hash TEXT NOT NULL, instance_id TEXT NOT NULL, state TEXT NOT NULL);`)
	if err != nil {
		return err
	}
	if err = m.initializeGuestUIDLeases(ctx); err != nil {
		return err
	}
	if err = m.validateGuestIdentityPolicy(ctx); err != nil {
		return err
	}
	for _, table := range []string{"runtime_instances", "runtime_stops"} {
		var columns int
		if err = m.Store.DB.QueryRowContext(ctx, "SELECT count(*) FROM pragma_table_info(?) WHERE name='revision'", table).Scan(&columns); err != nil {
			return err
		}
		if columns == 0 {
			if _, err = m.Store.DB.ExecContext(ctx, "ALTER TABLE "+table+" ADD COLUMN revision INTEGER NOT NULL DEFAULT 1"); err != nil {
				return err
			}
		}
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
	instance, err := m.inspectRuntimeRecord(ctx, id)
	if err != nil {
		return instance, err
	}
	if instance.State == "running" {
		domain := Domain{ID: instance.ID}
		domain.Image.SHA256, domain.Image.DataBytes = instance.ImageSHA256, instance.DataBytes
		if err := m.bindDomainGuestIdentity(ctx, &domain, false); err != nil {
			return Instance{}, err
		}
		instance.GuestUID = domain.GuestUID
	}
	return instance, nil
}

func (m *Manager) inspectRuntimeRecord(ctx context.Context, id string) (Instance, error) {
	var i Instance
	err := m.Store.DB.QueryRowContext(ctx, "SELECT r.id,r.workload,r.state,r.desired,r.image_sha256,r.memory_mib,r.vcpus,r.data_bytes,r.created_at,r.revision,coalesce(u.uid,0) FROM runtime_instances r LEFT JOIN runtime_uid_leases u ON u.instance_id=r.id WHERE r.id=?", id).Scan(&i.ID, &i.Workload, &i.State, &i.Desired, &i.ImageSHA256, &i.MemoryMiB, &i.VCPUs, &i.DataBytes, &i.CreatedAt, &i.Revision, &i.GuestUID)
	return i, err
}
func (m *Manager) Apply(ctx context.Context, r Request) (Instance, error) {
	unlock, err := m.lockRuntime(ctx, false)
	if err != nil {
		return Instance{}, err
	}
	defer unlock()
	if r.Version != 1 || !guestproto.ValidID(r.OperationID) || !guestproto.ValidID(r.InstanceID) || r.PolicyGeneration != m.Policy.Generation || r.Action != "inspect" && (r.Revision < 1 || r.Revision > 1<<53) {
		return Instance{}, ErrPolicy
	}
	switch r.Action {
	case "start":
		return m.start(ctx, r)
	case "stop":
		return m.stop(ctx, r)
	case "shutdown":
		return m.shutdown(ctx, r)
	case "inspect":
		return m.Inspect(ctx, r.InstanceID)
	case "purge":
		return m.purge(ctx, r)
	default:
		return Instance{}, ErrPolicy
	}
}
func (m *Manager) operation(tx *sql.Tx, r Request) (bool, error) {
	if err := requireRuntimeAdmission(tx); err != nil {
		return false, err
	}
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
	unlockStart, lockErr := m.lockStart(ctx)
	if lockErr != nil {
		return Instance{}, lockErr
	}
	defer unlockStart()
	if m.shuttingDown {
		return Instance{}, ErrPolicy
	}
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
		var existingRevision, existingDataBytes int64
		var existingMemoryMiB, existingVCPUs int
		existingErr := tx.QueryRow("SELECT workload,image_sha256,state,revision,memory_mib,vcpus,data_bytes FROM runtime_instances WHERE id=?", r.InstanceID).Scan(&existingWorkload, &existingDigest, &existingState, &existingRevision, &existingMemoryMiB, &existingVCPUs, &existingDataBytes)
		restarting := existingErr == nil
		var stopped int
		if e = tx.QueryRow("SELECT coalesce(max(revision),0) FROM runtime_stops WHERE instance_id=?", r.InstanceID).Scan(&stopped); e != nil {
			return e
		}
		if r.Revision <= int64(stopped) || restarting && r.Revision <= existingRevision {
			return ErrPolicy
		}
		if existingErr != nil && !errors.Is(existingErr, sql.ErrNoRows) {
			return existingErr
		}
		if restarting && (r.Workload == "video" || existingWorkload != r.Workload || existingDigest != image.SHA256 || existingMemoryMiB != image.MemoryMiB || existingVCPUs != image.VCPUs || existingDataBytes != image.DataBytes || existingState == "running" || existingState == "preparing" || existingState == "stopping" || existingState == "shutting-down") {
			return ErrPolicy
		}
		var count, memory, cpus int
		if e = tx.QueryRow("SELECT count(*),coalesce(sum(memory_mib+512),0),coalesce(sum(vcpus),0) FROM runtime_instances WHERE state IN('preparing','running','stopping','shutting-down')").Scan(&count, &memory, &cpus); e != nil {
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
		if e = tx.QueryRow("SELECT count(*) FROM runtime_instances WHERE workload=? AND (desired='running' OR state IN('preparing','running','stopping','shutting-down'))", r.Workload).Scan(&same); e != nil {
			return e
		}
		if r.Workload != "video" && same > 0 {
			return ErrCapacity
		}
		if restarting {
			_, e = tx.Exec("UPDATE runtime_instances SET state='preparing',desired='running',created_at=?,revision=? WHERE id=?", time.Now().Unix(), r.Revision, r.InstanceID)
		} else {
			_, e = tx.Exec("INSERT INTO runtime_instances VALUES(?,?,'preparing','running',?,?,?,?,?,?)", r.InstanceID, r.Workload, image.SHA256, image.MemoryMiB, image.VCPUs, image.DataBytes, time.Now().Unix(), r.Revision)
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
		stopErr := m.stopDomain(stopCtx, r.InstanceID)
		phase := "failed"
		if stopErr != nil {
			phase = "stopping"
		}
		journalCtx, cancelJournal := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelJournal()
		journalErr := m.Store.Transaction(journalCtx, func(tx *sql.Tx) error {
			if _, err := tx.Exec("UPDATE runtime_instances SET state=?,desired='stopped' WHERE id=?", phase, r.InstanceID); err != nil {
				return err
			}
			_, err := tx.Exec("UPDATE runtime_operations SET state='failed' WHERE id=?", r.OperationID)
			return err
		})
		return Instance{}, errors.Join(cause, stopErr, journalErr)
	}
	systemPath, err := catalog.VerifyImage(m.Images, image)
	if err != nil {
		return fail(err)
	}
	domain := Domain{ID: r.InstanceID, Image: image, DiskReserveBytes: m.Policy.DiskReserveBytes, SystemPath: systemPath, DataPath: filepath.Join(m.Volumes, r.InstanceID+".raw"), ChannelPath: filepath.Join(m.Channels, r.InstanceID, "adapter.sock")}
	if err = m.bindDomainGuestIdentity(ctx, &domain, true); err != nil {
		return fail(err)
	}
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
	if err = m.verifyPreparedDomain(ctx, domain, r.Revision); err != nil {
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
	var existed bool
	err := m.Store.Transaction(ctx, func(tx *sql.Tx) error {
		if _, err := m.operation(tx, r); err != nil {
			return err
		}
		var current int64
		if err := tx.QueryRow("SELECT coalesce(max(revision),0) FROM runtime_instances WHERE id=?", r.InstanceID).Scan(&current); err != nil {
			return err
		}
		if r.Revision < current {
			return ErrPolicy
		}
		existed = current != 0
		if _, err := tx.Exec("INSERT INTO runtime_stops VALUES(?,?) ON CONFLICT(instance_id) DO UPDATE SET revision=max(revision,excluded.revision)", r.InstanceID, r.Revision); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE runtime_instances SET desired='stopped',state='stopping',revision=? WHERE id=?", r.Revision, r.InstanceID)
		return err
	})
	if err != nil {
		return Instance{}, err
	}
	if err = m.stopDomain(ctx, r.InstanceID); err != nil {
		return Instance{}, err
	}
	err = m.Store.Transaction(ctx, func(tx *sql.Tx) error {
		var current int64
		if err := tx.QueryRow("SELECT coalesce(max(revision),0) FROM runtime_instances WHERE id=?", r.InstanceID).Scan(&current); err != nil {
			return err
		}
		if existed && current == 0 || current != 0 && current != r.Revision {
			return ErrPolicy
		}
		if current != 0 {
			result, err := tx.Exec("UPDATE runtime_instances SET state='stopped' WHERE id=? AND revision=? AND state='stopping' AND desired='stopped'", r.InstanceID, r.Revision)
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
		}
		result, err := tx.Exec("UPDATE runtime_operations SET state='succeeded' WHERE id=? AND instance_id=? AND state IN('pending','succeeded')", r.OperationID, r.InstanceID)
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
		return nil
	})
	if err != nil {
		return Instance{}, err
	}
	instance, err := m.Inspect(ctx, r.InstanceID)
	if errors.Is(err, sql.ErrNoRows) {
		return Instance{ID: r.InstanceID, State: "stopped", Desired: "stopped", Revision: r.Revision}, nil
	}
	return instance, err
}

// Reconcile never repeats an uncertain job. At startup it stops each recorded
// VM before marking interrupted operations; apps are restarted by new intent.
// Shutdown permanently closes start admission on this manager. Guest stop is
// a safety boundary, not an application-consistent backup operation.
func (m *Manager) Shutdown(ctx context.Context) error {
	unlock, err := m.lockRuntime(ctx, false)
	if err != nil {
		return err
	}
	defer unlock()
	unlockStart, err := m.lockStart(ctx)
	if err != nil {
		return err
	}
	defer unlockStart()
	m.shuttingDown = true
	return m.reconcile(ctx)
}

func (m *Manager) Reconcile(ctx context.Context) error {
	unlock, err := m.lockRuntime(ctx, false)
	if err != nil {
		return err
	}
	defer unlock()
	unlockStart, err := m.lockStart(ctx)
	if err != nil {
		return err
	}
	defer unlockStart()
	return m.reconcile(ctx)
}

func (m *Manager) reconcile(ctx context.Context) error {
	rows, err := m.Store.DB.QueryContext(ctx, "SELECT id FROM runtime_instances WHERE state IN('preparing','running','stopping','shutting-down')")
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
	var failures []error
	for _, id := range ids {
		if !guestproto.ValidID(id) {
			failures = append(failures, ErrPolicy)
			continue
		}
		if err = m.stopDomain(ctx, id); err != nil {
			failures = append(failures, fmt.Errorf("cannot stop uncertain instance: %w", err))
			continue
		}
		if _, err = m.Store.DB.ExecContext(ctx, "UPDATE runtime_instances SET state='interrupted',desired='stopped' WHERE id=?", id); err != nil {
			failures = append(failures, err)
		}
	}
	if len(failures) > 0 {
		return errors.Join(failures...)
	}
	_, err = m.Store.DB.ExecContext(ctx, "UPDATE runtime_operations SET state='interrupted' WHERE state='pending' AND id NOT IN(SELECT value FROM settings WHERE key='runtime.maintenance-job')")
	return err
}
func (m *Manager) Channel(ctx context.Context, id string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !guestproto.ValidID(id) {
		return "", ErrPolicy
	}
	instance, err := m.Inspect(ctx, id)
	if err != nil || instance.State != "running" || instance.Desired != "running" {
		return "", ErrPolicy
	}
	if instance.GuestUID != 0 {
		image, err := m.Manifest.Image(instance.Workload)
		if err != nil {
			return "", err
		}
		d := Domain{ID: id, Image: image}
		if err := m.bindDomainGuestIdentity(ctx, &d, false); err != nil {
			return "", err
		}
		if err := m.qualifyReservedChannelPath(ctx, d, instance.Revision); err != nil {
			return "", err
		}
	}
	path := filepath.Join(m.Channels, id, "adapter.sock")
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSocket == 0 {
		return "", ErrPolicy
	}
	current, err := m.Inspect(ctx, id)
	if err != nil || current != instance || current.State != "running" || current.Desired != "running" {
		return "", errors.Join(ErrPolicy, err)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return path, nil
}

// Audit enforces expiration, the finite-job wall clock and confinement even
// when the controller or guest adapter is unavailable or dishonest.
func (m *Manager) Audit(ctx context.Context) error {
	unlock, lockErr := m.lockRuntime(ctx, false)
	if lockErr != nil {
		return lockErr
	}
	defer unlock()
	rows, err := m.Store.DB.QueryContext(ctx, "SELECT id FROM runtime_instances WHERE state IN('running','stopping','shutting-down')")
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
		i, err := m.inspectRuntimeRecord(ctx, id)
		if err != nil {
			return err
		}
		if i.State != "running" && i.State != "stopping" && i.State != "shutting-down" {
			continue
		}
		image, imageErr := m.Manifest.Image(i.Workload)
		stop := hostErr != nil || imageErr != nil || i.ImageSHA256 != image.SHA256 || i.MemoryMiB != image.MemoryMiB || i.VCPUs != image.VCPUs || i.DataBytes != image.DataBytes || !m.Manifest.Expires.After(time.Now()) || i.State == "stopping" || i.Workload == "video" && time.Now().Unix()-i.CreatedAt >= 1800
		if i.State == "shutting-down" && !stop {
			var value string
			deadlineErr := m.Store.DB.QueryRowContext(ctx, "SELECT value FROM settings WHERE key=?", shutdownDeadlineKey(id)).Scan(&value)
			deadline, parseErr := strconv.ParseInt(value, 10, 64)
			stop = deadlineErr != nil || parseErr != nil || deadline <= time.Now().Unix() || deadline > time.Now().Unix()+90
			if !stop {
				running, probeErr := m.Backend.Running(ctx, id)
				if probeErr != nil {
					stop = true
				} else if !running {
					continue
				}
			}
		}
		if !stop {
			d := Domain{ID: id, Image: image, DiskReserveBytes: m.Policy.DiskReserveBytes, SystemPath: filepath.Join(m.Images, image.SHA256+".raw"), DataPath: filepath.Join(m.Volumes, id+".raw"), ChannelPath: filepath.Join(m.Channels, id, "adapter.sock")}
			stop = m.bindDomainGuestIdentity(ctx, &d, false) != nil
			if !stop {
				if i.State == "running" {
					stop = m.verifyRunningDomain(ctx, d, i.Revision) != nil
				} else {
					stop = m.verifyShuttingDownDomain(ctx, d, i.Revision) != nil
				}
			}
		}
		if stop {
			claimed, claimErr := m.Store.DB.ExecContext(ctx, "UPDATE runtime_instances SET state='stopping',desired='stopped' WHERE id=? AND revision=? AND state=? AND desired=?", id, i.Revision, i.State, i.Desired)
			if claimErr != nil {
				return claimErr
			}
			count, claimErr := claimed.RowsAffected()
			if claimErr != nil {
				return claimErr
			}
			if count == 0 {
				continue
			}
			if count != 1 {
				return ErrPolicy
			}
			if err = m.stopDomain(ctx, id); err != nil {
				return err
			}
			if _, err = m.Store.DB.ExecContext(ctx, "UPDATE runtime_instances SET state='interrupted' WHERE id=? AND revision=? AND state='stopping' AND desired='stopped'", id, i.Revision); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *Manager) purge(ctx context.Context, r Request) (Instance, error) {
	instance, err := m.Inspect(ctx, r.InstanceID)
	if err != nil {
		return Instance{}, err
	}
	// Reserved ownership requires inode-qualified deletion and durable intent
	// retirement. The legacy path-only purge cannot establish that authority.
	var reserved int
	if err := m.Store.DB.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM runtime_uid_leases WHERE instance_id=?)+(SELECT count(*) FROM runtime_guest_groups WHERE instance_id=?)+(SELECT count(*) FROM runtime_volume_ownership WHERE instance_id=?)+(SELECT count(*) FROM runtime_channel_ownership WHERE instance_id=?)+(SELECT count(*) FROM runtime_channel_sockets WHERE instance_id=?)`, r.InstanceID, r.InstanceID, r.InstanceID, r.InstanceID, r.InstanceID).Scan(&reserved); err != nil {
		return Instance{}, err
	}
	if reserved != 0 {
		return Instance{}, ErrPolicy
	}
	if instance.Workload != "video" || instance.Desired != "stopped" || instance.State == "running" || instance.State == "preparing" || instance.State == "stopping" || instance.State == "shutting-down" {
		return Instance{}, ErrPolicy
	}
	running, err := m.Backend.Running(ctx, r.InstanceID)
	if err != nil {
		return Instance{}, err
	}
	if running {
		return Instance{}, ErrPolicy
	}
	if err = m.Store.Transaction(ctx, func(tx *sql.Tx) error { _, err := m.operation(tx, r); return err }); err != nil {
		return Instance{}, err
	}
	if err := m.Backend.CleanupPreparation(ctx, m.Volumes, r.InstanceID, instance.DataBytes); err != nil {
		return Instance{}, err
	}
	for _, path := range []string{filepath.Join(m.Volumes, r.InstanceID+".raw"), filepath.Join(m.Channels, r.InstanceID, "adapter.sock"), filepath.Join(m.Channels, r.InstanceID)} {
		if err = os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return Instance{}, err
		}
	}
	if err = m.Store.Transaction(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE runtime_instances SET state='removed' WHERE id=?", r.InstanceID); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE runtime_operations SET state='succeeded' WHERE id=?", r.OperationID)
		return err
	}); err != nil {
		return Instance{}, err
	}
	return m.Inspect(ctx, r.InstanceID)
}
