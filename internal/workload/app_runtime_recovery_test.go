package workload

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/catalog"
	"github.com/pranavreddyg17/home-node/internal/state"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

// This fixture uses both production journals and managers, with no hypervisor.
type recoveryRuntime struct {
	running                  bool
	starts, stops, shutdowns int
}

func (b *recoveryRuntime) ValidateHost(context.Context, supervisor.Policy) error { return nil }
func (b *recoveryRuntime) Prepare(context.Context, supervisor.Domain) error      { return nil }
func (b *recoveryRuntime) Start(context.Context, supervisor.Domain) error {
	b.starts++
	b.running = true
	return nil
}
func (b *recoveryRuntime) Stop(context.Context, string) error {
	b.stops++
	b.running = false
	return nil
}
func (b *recoveryRuntime) Running(context.Context, string) (bool, error)   { return b.running, nil }
func (b *recoveryRuntime) Verify(context.Context, supervisor.Domain) error { return nil }
func (b *recoveryRuntime) FreeBytes(string) (int64, error)                 { return 100 * catalog.GiB, nil }
func (b *recoveryRuntime) CleanupPreparation(context.Context, string, string, int64) error {
	return nil
}
func (b *recoveryRuntime) Shutdown(context.Context, string) error {
	b.shutdowns++
	b.running = false
	return errors.New("fixture lost reply after exit")
}

type recoveryBoundary struct {
	*testBackend
	manager *supervisor.Manager
}

func (b *recoveryBoundary) Apply(ctx context.Context, r supervisor.Request) (supervisor.Instance, error) {
	return b.manager.Apply(ctx, r)
}

func TestControllerAndSupervisorRecoverLostShutdownCompletionTogether(t *testing.T) {
	s, guest, device := service(t)
	dir := t.TempDir()
	runtimeStore, err := state.Open(filepath.Join(dir, "runtime-journal"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtimeStore.Close() })
	image := []byte("fixture immutable image")
	digest := sum(image)
	if err = os.WriteFile(filepath.Join(dir, digest+".raw"), image, 0440); err != nil {
		t.Fatal(err)
	}
	runtime := &recoveryRuntime{}
	manager := &supervisor.Manager{Store: runtimeStore, Policy: supervisor.Policy{Generation: 1, MemoryMiB: 4096, VCPUs: 4, MaxInstances: 2, DiskReserveBytes: 4 * catalog.GiB, ControllerUID: 1001, TransferUID: 1002}, Manifest: catalog.Manifest{Schema: 1, Version: 1, Expires: time.Now().Add(time.Hour), Images: []catalog.Image{{ID: "files", SHA256: digest, Bytes: int64(len(image)), MemoryMiB: 512, VCPUs: 1, DataBytes: catalog.GiB, Protocol: 1}}}, Images: dir, Volumes: dir, Channels: dir, Backend: runtime}
	ctx := context.Background()
	if err = manager.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	s.Backend = &recoveryBoundary{testBackend: guest, manager: manager}
	startFiles(t, s, device)
	data := []byte("retained file across recovered shutdown")
	transfer, err := s.CreateTransfer(ctx, device, "retained.txt", int64(len(data)), sum(data))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Upload(ctx, device, transfer.ID, 0, data, sum(data)); err != nil {
		t.Fatal(err)
	}
	file, err := s.Finalize(ctx, device, transfer.ID)
	if err != nil {
		t.Fatal(err)
	}
	key := state.Random()
	stop, err := s.AppAction(ctx, device, key, "files", "stop")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.processApp(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.observeUncertainApps(ctx); err != nil {
		t.Fatal(err)
	}
	apps, err := s.Apps(ctx)
	if err != nil || len(apps) != 1 || apps[0].State != "stopping" || !apps[0].RequiresAttention {
		t.Fatal("unconfirmed supervisor state cleared controller fence", apps, err)
	}
	if err = manager.Audit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.observeUncertainApps(ctx); err != nil {
		t.Fatal(err)
	}
	apps, err = s.Apps(ctx)
	if err != nil || apps[0].State != "stopped" || apps[0].RequiresAttention {
		t.Fatal("journals failed to converge", apps, err)
	}
	replay, err := s.AppAction(ctx, device, key, "files", "stop")
	if err != nil || replay.ID != stop.ID || replay.State != "interrupted" {
		t.Fatal("controller replay changed recovered outcome", replay, err)
	}
	var nativeState string
	if err = runtimeStore.DB.QueryRow("SELECT state FROM runtime_operations WHERE id=?", stop.ID).Scan(&nativeState); err != nil || nativeState != "succeeded" {
		t.Fatal("supervisor receipt did not complete", nativeState, err)
	}
	if runtime.starts != 1 || runtime.stops != 0 || runtime.shutdowns != 1 {
		t.Fatal("recovery dispatched extra runtime effects", runtime)
	}
	startFiles(t, s, device)
	if runtime.starts != 2 || runtime.stops != 0 || runtime.shutdowns != 1 {
		t.Fatal("recovered app could not restart explicitly", runtime)
	}
	retained, err := s.Download(ctx, file.ID, 0)
	if err != nil || string(retained) != string(data) {
		t.Fatal("recovery lost retained guest data", string(retained), err)
	}
}
