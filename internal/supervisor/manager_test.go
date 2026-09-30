package supervisor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/catalog"
	"github.com/pranavreddyg17/home-node/internal/state"
)

type fakeBackend struct {
	starts, stops                  int
	hostErr, verifyErr, cleanupErr error
	running                        bool
	free                           int64
}

func (b *fakeBackend) ValidateHost(context.Context, Policy) error    { return b.hostErr }
func (b *fakeBackend) Prepare(context.Context, Domain) error         { return nil }
func (b *fakeBackend) Start(context.Context, Domain) error           { b.starts++; b.running = true; return nil }
func (b *fakeBackend) Stop(context.Context, string) error            { b.stops++; b.running = false; return nil }
func (b *fakeBackend) Running(context.Context, string) (bool, error) { return b.running, nil }
func (b *fakeBackend) Verify(context.Context, Domain) error          { return b.verifyErr }
func (b *fakeBackend) FreeBytes(string) (int64, error)               { return b.free, nil }
func (b *fakeBackend) CleanupPreparation(context.Context, string, string, int64) error {
	return b.cleanupErr
}
func newManager(t *testing.T) (*Manager, *fakeBackend) {
	t.Helper()
	dir := t.TempDir()
	store, err := state.Open(filepath.Join(dir, "journal"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	image := []byte("trusted image")
	digest := sha256.Sum256(image)
	hash := hex.EncodeToString(digest[:])
	if err = os.WriteFile(filepath.Join(dir, hash+".raw"), image, 0440); err != nil {
		t.Fatal(err)
	}
	backend := &fakeBackend{free: 100 * catalog.GiB}
	m := &Manager{Store: store, Policy: Policy{Generation: 1, MemoryMiB: 4096, VCPUs: 4, MaxInstances: 2, DiskReserveBytes: 4 * catalog.GiB, ControllerUID: 1001, TransferUID: 1002}, Manifest: catalog.Manifest{Schema: 1, Version: 1, Expires: time.Now().Add(time.Hour), Images: []catalog.Image{{ID: "files", SHA256: hash, Bytes: int64(len(image)), MemoryMiB: 512, VCPUs: 1, DataBytes: catalog.GiB, Protocol: 1}}}, Images: dir, Volumes: dir, Channels: dir, Backend: backend}
	if err = m.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	return m, backend
}
func startRequest() Request {
	return Request{Version: 1, OperationID: state.Random(), InstanceID: state.Random(), Action: "start", Revision: 1, Workload: "files", PolicyGeneration: 1}
}

type failingStopBackend struct {
	*fakeBackend
	failID string
}

func (b *failingStopBackend) Stop(ctx context.Context, id string) error {
	if id == b.failID {
		b.stops++
		return errors.New("stop failed")
	}
	return b.fakeBackend.Stop(ctx, id)
}

func TestShutdownStopsAllAndKeepsFailedStopVisible(t *testing.T) {
	m, b := newManager(t)
	ctx := context.Background()
	first, second := startRequest(), startRequest()
	ai := m.Manifest.Images[0]
	ai.ID = "ai"
	m.Manifest.Images = append(m.Manifest.Images, ai)
	second.Workload = "ai"
	for _, r := range []Request{first, second} {
		if _, err := m.Apply(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	failing := &failingStopBackend{fakeBackend: b, failID: first.InstanceID}
	m.Backend = failing
	if err := m.Shutdown(ctx); err == nil {
		t.Fatal("stop failure hidden")
	}
	if b.stops != 2 {
		t.Fatal("one failed stop skipped another guest", b.stops)
	}
	one, err := m.Inspect(ctx, first.InstanceID)
	if err != nil || one.State != "running" {
		t.Fatal("failed stop falsely completed", one, err)
	}
	two, err := m.Inspect(ctx, second.InstanceID)
	if err != nil || two.State != "interrupted" || two.Desired != "stopped" {
		t.Fatal(two, err)
	}
	if _, err = m.Apply(ctx, startRequest()); !errors.Is(err, ErrPolicy) {
		t.Fatal("shutdown admitted start", err)
	}
	failing.failID = ""
	if err = m.Shutdown(ctx); err != nil {
		t.Fatal("retry failed", err)
	}
	one, err = m.Inspect(ctx, first.InstanceID)
	if err != nil || one.State != "interrupted" {
		t.Fatal(one, err)
	}
}
func TestStartReplayAndConflict(t *testing.T) {
	m, b := newManager(t)
	ctx := context.Background()
	r := startRequest()
	i, err := m.Apply(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if i.State != "running" {
		t.Fatal(i.State)
	}
	if _, err = m.Apply(ctx, r); err != nil {
		t.Fatal(err)
	}
	if b.starts != 1 {
		t.Fatal("idempotent request started twice")
	}
	r.InstanceID = state.Random()
	if _, err = m.Apply(ctx, r); !errors.Is(err, ErrPolicy) {
		t.Fatalf("changed request accepted: %v", err)
	}
}
func TestIsolationFailureStopsVM(t *testing.T) {
	m, b := newManager(t)
	b.verifyErr = errors.New("unconfined")
	r := startRequest()
	if _, err := m.Apply(context.Background(), r); err == nil {
		t.Fatal("failed confinement accepted")
	}
	if b.running || b.stops != 1 {
		t.Fatal("unconfined guest remained running")
	}
	i, err := m.Inspect(context.Background(), r.InstanceID)
	if err != nil || i.State != "failed" {
		t.Fatalf("wrong failed state: %+v %v", i, err)
	}
}
func TestPolicyAndCapacityFailBeforeStart(t *testing.T) {
	for _, kind := range []string{"host", "memory", "disk", "generation"} {
		t.Run(kind, func(t *testing.T) {
			m, b := newManager(t)
			r := startRequest()
			switch kind {
			case "host":
				b.hostErr = errors.New("unsupported")
			case "memory":
				m.Policy.MemoryMiB = 512
			case "disk":
				b.free = 0
			case "generation":
				r.PolicyGeneration = 0
			}
			if _, err := m.Apply(context.Background(), r); err == nil {
				t.Fatal("invalid admission accepted")
			}
			if b.starts != 0 {
				t.Fatal("guest started before admission")
			}
		})
	}
}
func TestRestartReconciliationDoesNotRerunJob(t *testing.T) {
	m, b := newManager(t)
	r := startRequest()
	if _, err := m.Apply(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	i, err := m.Inspect(context.Background(), r.InstanceID)
	if err != nil {
		t.Fatal(err)
	}
	if i.State != "interrupted" || b.running || b.starts != 1 {
		t.Fatal("uncertain execution was repeated")
	}
}
func TestDomainHasOnlyApprovedDevices(t *testing.T) {
	m, _ := newManager(t)
	image := m.Manifest.Images[0]
	d := Domain{ID: state.Random(), Image: image, SystemPath: "/images/base.raw", DataPath: "/volumes/data.raw", ChannelPath: "/run/channel.sock"}
	text, err := d.XML()
	if err != nil {
		t.Fatal(err)
	}
	decoder := xml.NewDecoder(strings.NewReader(text))
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		if element, ok := token.(xml.StartElement); ok {
			for _, forbidden := range []string{"interface", "hostdev", "filesystem", "graphics", "commandline", "vsock"} {
				if element.Name.Local == forbidden {
					t.Fatalf("unexpected %s", forbidden)
				}
			}
		}
	}
	for _, required := range []string{"model='apparmor'", "<readonly/>", "<hard_limit", "<quota>", "type='raw'", "<partition>/homenode</partition>"} {
		if !strings.Contains(text, required) {
			t.Fatalf("missing %s", required)
		}
	}
	var parsed struct {
		Disks []struct {
			Serial string `xml:"serial"`
			Target struct {
				Dev string `xml:"dev,attr"`
			} `xml:"target"`
			Source struct {
				File string `xml:"file,attr"`
			} `xml:"source"`
			ReadOnly *struct{} `xml:"readonly"`
		} `xml:"devices>disk"`
	}
	if err := xml.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Disks) != 2 {
		t.Fatalf("unexpected disk count: %d", len(parsed.Disks))
	}
	system, data := parsed.Disks[0], parsed.Disks[1]
	if system.Serial != "homenode-system" || system.Target.Dev != "vda" || system.Source.File != d.SystemPath || system.ReadOnly == nil {
		t.Fatal("system disk role is not uniquely bound to its read-only source")
	}
	if data.Serial != "homenode-data" || data.Target.Dev != "vdb" || data.Source.File != d.DataPath || data.ReadOnly != nil {
		t.Fatal("data disk role is not uniquely bound to its writable source")
	}
	d.ID = "../../inject"
	if _, err = d.XML(); err == nil {
		t.Fatal("unsafe domain id accepted")
	}
}
func FuzzDomainID(f *testing.F) {
	f.Add("safe_resource_id_123456")
	f.Add("<interface/>")
	f.Fuzz(func(t *testing.T, id string) {
		d := Domain{ID: id, Image: catalog.Image{MemoryMiB: 512, VCPUs: 1}, SystemPath: "/images/base.raw", DataPath: "/volumes/data.raw", ChannelPath: "/run/channel.sock"}
		text, err := d.XML()
		if err != nil {
			return
		}
		var parsed struct{ XMLName xml.Name }
		if err = xml.Unmarshal([]byte(text), &parsed); err != nil {
			t.Fatal(err)
		}
		if parsed.XMLName.Local != "domain" {
			t.Fatal("unexpected XML root")
		}
	})
}

func TestPolicyRollbackAndUnversionedChange(t *testing.T) {
	m, _ := newManager(t)
	m.Policy.VCPUs++
	if err := m.Initialize(context.Background()); !errors.Is(err, ErrPolicy) {
		t.Fatalf("unversioned policy accepted: %v", err)
	}
	m.Policy.Generation++
	if err := m.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	m.Policy.Generation--
	if err := m.Initialize(context.Background()); !errors.Is(err, ErrPolicy) {
		t.Fatalf("policy rollback accepted: %v", err)
	}
}
func TestPersistentAppRestartKeepsVolumeIdentity(t *testing.T) {
	m, b := newManager(t)
	ctx := context.Background()
	r := startRequest()
	if _, err := m.Apply(ctx, r); err != nil {
		t.Fatal(err)
	}
	stop := r
	stop.Action = "stop"
	stop.Revision = 2
	stop.OperationID = state.Random()
	if _, err := m.Apply(ctx, stop); err != nil {
		t.Fatal(err)
	}
	r.OperationID = state.Random()
	r.Revision = 3
	i, err := m.Apply(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if i.ID != r.InstanceID || i.State != "running" || b.starts != 2 {
		t.Fatalf("incorrect restart: %+v", i)
	}
}
func TestAuditStopsExpiredCatalog(t *testing.T) {
	m, b := newManager(t)
	r := startRequest()
	if _, err := m.Apply(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	m.Manifest.Expires = time.Now().Add(-time.Minute)
	if err := m.Audit(context.Background()); err != nil {
		t.Fatal(err)
	}
	if b.running {
		t.Fatal("expired catalog remained running")
	}
	i, err := m.Inspect(context.Background(), r.InstanceID)
	if err != nil || i.State != "interrupted" {
		t.Fatalf("audit state %+v %v", i, err)
	}
}

func TestStopBeforeStartPreventsLateLaunch(t *testing.T) {
	m, b := newManager(t)
	r := startRequest()
	stop := r
	stop.OperationID = state.Random()
	stop.Action = "stop"
	stop.Revision = 2
	if _, err := m.Apply(context.Background(), stop); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Apply(context.Background(), r); !errors.Is(err, ErrPolicy) {
		t.Fatalf("late start accepted: %v", err)
	}
	if b.starts != 0 {
		t.Fatal("cancelled VM launched")
	}
}

func TestOldLifecycleIntentCannotUndoNewerStopOrStart(t *testing.T) {
	m, b := newManager(t)
	ctx := context.Background()
	start := startRequest()
	if _, err := m.Apply(ctx, start); err != nil {
		t.Fatal(err)
	}
	stop := start
	stop.Action = "stop"
	stop.OperationID = state.Random()
	stop.Revision = 2
	if _, err := m.Apply(ctx, stop); err != nil {
		t.Fatal(err)
	}
	late := start
	late.OperationID = state.Random()
	if _, err := m.Apply(ctx, late); !errors.Is(err, ErrPolicy) {
		t.Fatalf("stale start accepted: %v", err)
	}
	newStart := start
	newStart.OperationID = state.Random()
	newStart.Revision = 3
	if _, err := m.Apply(ctx, newStart); err != nil {
		t.Fatal(err)
	}
	lateStop := stop
	lateStop.OperationID = state.Random()
	if _, err := m.Apply(ctx, lateStop); !errors.Is(err, ErrPolicy) {
		t.Fatalf("stale stop accepted: %v", err)
	}
	if !b.running {
		t.Fatal("stale stop killed newer authorized instance")
	}
}

func TestPurgeWaitsForPreparationCleanup(t *testing.T) {
	m, b := newManager(t)
	id := state.Random()
	_, err := m.Store.DB.Exec("INSERT INTO runtime_instances(id,workload,state,desired,image_sha256,memory_mib,vcpus,data_bytes,created_at,revision) VALUES(?,'video','failed','stopped',?,512,1,?,0,1)", id, m.Manifest.Images[0].SHA256, catalog.GiB)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(m.Volumes, id+".raw")
	if err := os.WriteFile(path, []byte("owner data"), 0600); err != nil {
		t.Fatal(err)
	}
	b.cleanupErr = errors.New("staging cleanup failed")
	request := Request{Version: 1, OperationID: state.Random(), InstanceID: id, Action: "purge", Revision: 3, PolicyGeneration: 1}
	if _, err := m.Apply(context.Background(), request); err == nil {
		t.Fatal("cleanup failure hidden")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "owner data" {
		t.Fatal("failed cleanup erased published volume")
	}
	instance, err := m.Inspect(context.Background(), id)
	if err != nil || instance.State == "removed" {
		t.Fatal("failed cleanup marked removed")
	}
	var phase string
	if err := m.Store.DB.QueryRow("SELECT state FROM runtime_operations WHERE id=?", request.OperationID).Scan(&phase); err != nil || phase != "pending" {
		t.Fatal("failed cleanup completed operation", phase, err)
	}
	b.cleanupErr = nil
	instance, err = m.Apply(context.Background(), request)
	if err != nil || instance.State != "removed" {
		t.Fatal("cleanup replay did not finish", instance, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("successful purge retained published volume")
	}
}
