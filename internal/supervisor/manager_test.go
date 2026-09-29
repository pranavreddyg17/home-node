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
	starts, stops      int
	hostErr, verifyErr error
	running            bool
	free               int64
}

func (b *fakeBackend) ValidateHost(context.Context, Policy) error    { return b.hostErr }
func (b *fakeBackend) Prepare(context.Context, Domain) error         { return nil }
func (b *fakeBackend) Start(context.Context, Domain) error           { b.starts++; b.running = true; return nil }
func (b *fakeBackend) Stop(context.Context, string) error            { b.stops++; b.running = false; return nil }
func (b *fakeBackend) Running(context.Context, string) (bool, error) { return b.running, nil }
func (b *fakeBackend) Verify(context.Context, Domain) error          { return b.verifyErr }
func (b *fakeBackend) FreeBytes(string) (int64, error)               { return b.free, nil }
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
	return Request{Version: 1, OperationID: state.Random(), InstanceID: state.Random(), Action: "start", Workload: "files", PolicyGeneration: 1}
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
	stop.OperationID = state.Random()
	if _, err := m.Apply(ctx, stop); err != nil {
		t.Fatal(err)
	}
	r.OperationID = state.Random()
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
