package workload

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/guest"
	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"github.com/pranavreddyg17/home-node/internal/state"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

// The test backend exercises the real guest byte store, but cannot establish
// a hypervisor boundary. Production has no in-process workload backend.
type testBackend struct {
	agent   *guest.Agent
	starts  int
	running bool
}

func (b *testBackend) Apply(_ context.Context, r supervisor.Request) (supervisor.Instance, error) {
	if r.Action == "start" {
		b.starts++
		b.running = true
	}
	if r.Action == "stop" {
		b.running = false
	}
	phase := "stopped"
	if b.running {
		phase = "running"
	}
	return supervisor.Instance{ID: r.InstanceID, State: phase, Workload: r.Workload}, nil
}
func (b *testBackend) Call(_ context.Context, _ string, r guestproto.Request) (guestproto.Response, error) {
	return b.agent.Handle(r), nil
}
func service(t *testing.T) (*Service, *testBackend, string) {
	t.Helper()
	store, err := state.Open(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	agent, err := guest.New(filepath.Join(t.TempDir(), "data"), "files", 16<<30)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close(); _ = agent.Close() })
	id := state.Random()
	if _, err = store.DB.Exec("INSERT INTO devices(id,name,capabilities,created_at) VALUES(?,'test','[\"admin\",\"files\"]',?)", id, time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	backend := &testBackend{agent: agent}
	s := New(store, backend, 1)
	return s, backend, id
}
func startFiles(t *testing.T, s *Service, device string) {
	t.Helper()
	if _, err := s.AppAction(context.Background(), device, state.Random(), "files", "start"); err != nil {
		t.Fatal(err)
	}
	if err := s.processApp(context.Background()); err != nil {
		t.Fatal(err)
	}
}
func TestUploadDownloadTrashRestore(t *testing.T) {
	s, _, device := service(t)
	ctx := context.Background()
	startFiles(t, s, device)
	data := []byte("file content survives the full boundary protocol")
	transfer, err := s.CreateTransfer(ctx, device, "test.txt", int64(len(data)), sum(data))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Transfer(ctx, "another-device", transfer.ID); err == nil {
		t.Fatal("transfer crossed device scope")
	}
	if _, err = s.Upload(ctx, device, transfer.ID, 0, data[:10], sum(data[:10])); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Finalize(ctx, device, transfer.ID); err == nil {
		t.Fatal("unfinished input committed")
	}
	if _, err = s.Upload(ctx, device, transfer.ID, 10, data[10:], sum(data[10:])); err != nil {
		t.Fatal(err)
	}
	file, err := s.Finalize(ctx, device, transfer.ID)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := s.Download(ctx, file.ID, 0)
	if err != nil || string(actual) != string(data) {
		t.Fatalf("bad download %q %v", actual, err)
	}
	if err = s.ChangeFile(ctx, device, file.ID, "trash", ""); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Download(ctx, file.ID, 0); err == nil {
		t.Fatal("trashed content accessible")
	}
	if err = s.ChangeFile(ctx, device, file.ID, "restore", ""); err != nil {
		t.Fatal(err)
	}
	if err = s.ChangeFile(ctx, device, file.ID, "rename", "renamed.txt"); err != nil {
		t.Fatal(err)
	}
	items, err := s.Files(ctx)
	if err != nil || len(items) != 1 || items[0].Name != "renamed.txt" {
		t.Fatalf("wrong files %+v %v", items, err)
	}
}
func TestAppIntentDeduplicatesAndConflicts(t *testing.T) {
	s, b, device := service(t)
	ctx := context.Background()
	key := state.Random()
	first, err := s.AppAction(ctx, device, key, "files", "start")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.AppAction(ctx, device, key, "files", "start")
	if err != nil || first.ID != second.ID {
		t.Fatalf("intent duplicated %v", err)
	}
	if _, err = s.AppAction(ctx, device, key, "ai", "start"); err == nil {
		t.Fatal("changed idempotent intent accepted")
	}
	if err = s.processApp(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.processApp(ctx); err != nil {
		t.Fatal(err)
	}
	if b.starts != 1 {
		t.Fatal("duplicate side effect")
	}
	op, err := s.Operation(ctx, device, first.ID)
	if err != nil || op.State != "succeeded" {
		t.Fatal(op, err)
	}
	if _, err = s.Operation(ctx, "other", first.ID); err == nil {
		t.Fatal("operation crossed device scope")
	}
}
func TestRevokedQueuedIntentCannotStart(t *testing.T) {
	s, b, device := service(t)
	ctx := context.Background()
	op, err := s.AppAction(ctx, device, state.Random(), "files", "start")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Store.DB.Exec("UPDATE devices SET revoked_at=?", time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	if err = s.processApp(ctx); err != nil {
		t.Fatal(err)
	}
	if b.starts != 0 {
		t.Fatal("revoked actor started workload")
	}
	result, err := s.Operation(ctx, device, op.ID)
	if err != nil || result.State != "failed" {
		t.Fatal(result, err)
	}
	var body map[string]string
	if err = json.Unmarshal(result.Result, &body); err != nil {
		t.Fatal(err)
	}
	if body["code"] != "AUTHORIZATION_EXPIRED" {
		t.Fatal(body)
	}
}
func TestReservationAndChunkIntegrity(t *testing.T) {
	s, _, device := service(t)
	ctx := context.Background()
	startFiles(t, s, device)
	for i := 0; i < 12; i++ {
		if _, err := s.CreateTransfer(ctx, device, "large.bin", MaxFileBytes, sum(nil)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.CreateTransfer(ctx, device, "too-much.bin", 1, sum(nil)); err == nil {
		t.Fatal("overbooked disk quota")
	}
	if _, err := s.CreateTransfer(ctx, device, "../escape", 0, sum(nil)); err == nil {
		t.Fatal("unsafe name accepted")
	}
}

func TestEmptyFile(t *testing.T) {
	s, _, device := service(t)
	ctx := context.Background()
	startFiles(t, s, device)
	transfer, err := s.CreateTransfer(ctx, device, "empty.txt", 0, sum(nil))
	if err != nil {
		t.Fatal(err)
	}
	file, err := s.Finalize(ctx, device, transfer.ID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := s.Download(ctx, file.ID, 0)
	if err != nil || len(data) != 0 {
		t.Fatalf("empty download %v", err)
	}
}

func TestAppIntentParticipatesInCallerTransaction(t *testing.T) {
	s, backend, device := service(t)
	failed := errors.New("approval transaction failure")
	err := s.Store.Transaction(context.Background(), func(tx *sql.Tx) error {
		if _, err := s.AppActionInTransaction(tx, device, state.Random(), "files", "start"); err != nil {
			return err
		}
		return failed
	})
	if !errors.Is(err, failed) {
		t.Fatal(err)
	}
	for _, table := range []string{"apps", "operations"} {
		var count int
		if err := s.Store.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatal("rolled back intent survived", table, count, err)
		}
	}
	if backend.starts != 0 {
		t.Fatal("transaction executed runtime effect")
	}
	err = s.Store.Transaction(context.Background(), func(tx *sql.Tx) error {
		_, err := s.AppActionInTransaction(tx, device, state.Random(), "files", "start")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.Store.DB.QueryRow("SELECT count(*) FROM operations WHERE state='pending'").Scan(&count); err != nil || count != 1 {
		t.Fatal("committed intent missing", count, err)
	}
	if backend.starts != 0 {
		t.Fatal("committed intent bypassed worker")
	}
}
