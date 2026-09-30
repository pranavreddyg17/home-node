package workload

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/guest"
	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"github.com/pranavreddyg17/home-node/internal/state"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

// This fixture verifies orchestration and byte integrity. The canned conversion
// result does not test FFmpeg or hardware isolation; Linux guest tests must.
type jobBackend struct {
	files, video        *guest.Agent
	videoID             string
	runs, stops, purges int
	corrupt             bool
	onRun               func()
	output              []byte
}

func (b *jobBackend) Apply(_ context.Context, r supervisor.Request) (supervisor.Instance, error) {
	if r.Action == "start" && r.Workload == "video" {
		b.videoID = r.InstanceID
	}
	phase := "running"
	if r.Action == "stop" || r.Action == "shutdown" {
		b.stops++
		phase = "stopped"
	}
	if r.Action == "purge" {
		b.purges++
		phase = "removed"
	}
	return supervisor.Instance{ID: r.InstanceID, State: phase, Workload: r.Workload, Revision: r.Revision}, nil
}
func (b *jobBackend) Call(_ context.Context, id string, r guestproto.Request) (guestproto.Response, error) {
	agent := b.files
	if id == b.videoID {
		agent = b.video
	}
	switch r.Operation {
	case "run":
		b.runs++
		for _, req := range []guestproto.Request{{Operation: "upload", Size: int64(len(b.output)), Data: b.output, SHA256: sum(b.output)}, {Operation: "finalize", Size: int64(len(b.output)), SHA256: sum(b.output)}} {
			req.Version = 1
			req.RequestID = state.Random()
			req.ObjectID = r.ObjectID
			if response := agent.Handle(req); response.Error != "" {
				return response, ErrConflict
			}
		}
		if b.onRun != nil {
			b.onRun()
		}
		return guestproto.Response{Version: 1, RequestID: r.RequestID, State: "running"}, nil
	case "result":
		hash := sum(b.output)
		if b.corrupt {
			hash = sum([]byte("different"))
		}
		return guestproto.Response{Version: 1, RequestID: r.RequestID, State: "succeeded", Size: int64(len(b.output)), SHA256: hash}, nil
	}
	return agent.Handle(r), nil
}
func jobService(t *testing.T) (*Service, *jobBackend, string, File) {
	t.Helper()
	s, original, device := service(t)
	startFiles(t, s, device)
	video, err := guest.New(filepath.Join(t.TempDir(), "video"), "video", 8<<30)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = video.Close() })
	backend := &jobBackend{files: original.agent, video: video, output: []byte("converted output fixture")}
	s.Backend = backend
	if _, err = s.Store.DB.Exec("UPDATE devices SET capabilities='[\"admin\",\"files\",\"jobs\"]'"); err != nil {
		t.Fatal(err)
	}
	data := []byte("video input fixture")
	transfer, err := s.CreateTransfer(context.Background(), device, "input.mp4", int64(len(data)), sum(data))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Upload(context.Background(), device, transfer.ID, 0, data, sum(data)); err != nil {
		t.Fatal(err)
	}
	file, err := s.Finalize(context.Background(), device, transfer.ID)
	if err != nil {
		t.Fatal(err)
	}
	return s, backend, device, file
}
func TestJobCopiesVerifiesCommitsAndStops(t *testing.T) {
	s, b, device, input := jobService(t)
	ctx := context.Background()
	key := state.Random()
	j, err := s.CreateJob(ctx, device, key, input.ID, "mp4-720p", "")
	if err != nil {
		t.Fatal(err)
	}
	same, err := s.CreateJob(ctx, device, key, input.ID, "mp4-720p", "")
	if err != nil || same.AttemptID != j.AttemptID {
		t.Fatal("duplicate job intent", err)
	}
	if err = s.processJob(ctx); err != nil {
		t.Fatal(err)
	}
	result, err := s.attempt(ctx, j.AttemptID)
	if err != nil || result.State != "succeeded" || result.OutputID == nil {
		t.Fatalf("result %+v %v", result, err)
	}
	actual, err := s.Download(ctx, *result.OutputID, 0)
	if err != nil || string(actual) != string(b.output) {
		t.Fatalf("output integrity %q %v", actual, err)
	}
	if b.runs != 1 || b.stops != 1 || b.purges != 1 {
		t.Fatalf("lifecycle runs=%d stops=%d purges=%d", b.runs, b.stops, b.purges)
	}
	if err = s.processJob(ctx); err != nil {
		t.Fatal(err)
	}
	if b.runs != 1 {
		t.Fatal("completed job rerun")
	}
}
func TestCorruptOutputNeverSucceeds(t *testing.T) {
	s, b, device, input := jobService(t)
	b.corrupt = true
	j, err := s.CreateJob(context.Background(), device, state.Random(), input.ID, "mp4-720p", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.processJob(context.Background()); err != nil {
		t.Fatal(err)
	}
	result, err := s.attempt(context.Background(), j.AttemptID)
	if err != nil || result.State != "failed" || result.OutputID != nil {
		t.Fatalf("corrupt output committed %+v %v", result, err)
	}
	files, err := s.Files(context.Background())
	if err != nil || len(files) != 1 {
		t.Fatal("partial result exposed", err)
	}
}
func TestCancellationAndExplicitRetry(t *testing.T) {
	s, b, device, input := jobService(t)
	ctx := context.Background()
	j, err := s.CreateJob(ctx, device, state.Random(), input.ID, "mp4-720p", "")
	if err != nil {
		t.Fatal(err)
	}
	b.onRun = func() {
		if err := s.CancelJob(ctx, device, j.ID, j.AttemptID); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.processJob(ctx); err != nil {
		t.Fatal(err)
	}
	cancelled, err := s.attempt(ctx, j.AttemptID)
	if err != nil || cancelled.State != "cancelled" {
		t.Fatalf("cancel %+v %v", cancelled, err)
	}
	retry, err := s.CreateJob(ctx, device, state.Random(), input.ID, "mp4-720p", j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if retry.ID != j.ID || retry.AttemptID == j.AttemptID || retry.InstanceID == j.InstanceID {
		t.Fatal("retry reused an uncertain attempt or VM")
	}
}
func TestRestartDoesNotRepeatUncertainJob(t *testing.T) {
	s, b, device, input := jobService(t)
	ctx := context.Background()
	j, err := s.CreateJob(ctx, device, state.Random(), input.ID, "mp4-720p", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Store.DB.Exec("UPDATE jobs SET state='running',start_requested=1 WHERE id=?", j.AttemptID); err != nil {
		t.Fatal(err)
	}
	if err = s.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	result, err := s.attempt(ctx, j.AttemptID)
	if err != nil || result.State != "interrupted" || b.runs != 0 || b.stops != 1 {
		t.Fatalf("restart %+v %v", result, err)
	}
}
