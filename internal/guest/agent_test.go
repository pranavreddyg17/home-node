package guest

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestDeleteRemovesInterruptedJournalAndPreservesOtherObjects(t *testing.T) {
	a, err := New(privateDataDir(t), "ai", 1<<30)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	id, other := state.Random(), state.Random()
	suffixes := []string{".part", ".blob", ".task.json", ".task.tmp", ".working"}
	for _, suffix := range suffixes {
		if err = a.root.WriteFile(id+suffix, []byte("interrupted object bytes"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err = a.root.WriteFile(other+".task.tmp", []byte("unrelated task"), 0600); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		response := a.Handle(guestproto.Request{Version: 1, RequestID: state.Random(), Operation: "delete", ObjectID: id})
		if response.Error != "" {
			t.Fatal("delete/replay failed", response)
		}
		for _, suffix := range suffixes {
			if _, err = a.root.Stat(id + suffix); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("deleted object retained data", suffix, err)
			}
		}
		data, err := a.root.ReadFile(other + ".task.tmp")
		if err != nil || string(data) != "unrelated task" {
			t.Fatal("unrelated object was modified", err)
		}
	}
}

func TestResumableIntegrityWorkflow(t *testing.T) {
	dir := filepath.Join(privateDataDir(t), "objects")
	a, err := New(dir, "files", 1<<30)
	if err != nil {
		t.Fatal(err)
	}
	id := state.Random()
	data := []byte("first chunk / second chunk")
	send := func(op string, offset int64, bytes []byte, hash string) guestproto.Response {
		return a.Handle(guestproto.Request{Version: 1, RequestID: state.Random(), Operation: op, ObjectID: id, Offset: offset, Size: int64(len(data)), Data: bytes, SHA256: hash})
	}
	first := send("upload", 0, data[:12], checksum(data[:12]))
	if first.Error != "" || first.Offset != 12 {
		t.Fatalf("first: %+v", first)
	}
	replay := send("upload", 0, data[:12], checksum(data[:12]))
	if replay.Error != "" || replay.Offset != 12 {
		t.Fatalf("replay: %+v", replay)
	}
	conflict := send("upload", 0, []byte("wrong"), checksum([]byte("wrong")))
	if conflict.Error == "" {
		t.Fatal("changed chunk accepted")
	}
	a.Close()
	a, err = New(dir, "files", 1<<30)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if got := send("upload", 12, data[12:], checksum(data[12:])); got.Error != "" {
		t.Fatalf("resume: %+v", got)
	}
	if got := send("finalize", 0, nil, "bad hash"); got.Error == "" {
		t.Fatal("incorrect object digest accepted")
	}
	if got := send("download", 0, nil, ""); got.Error == "" {
		t.Fatal("unfinished bytes downloadable")
	}
	if got := send("finalize", 0, nil, checksum(data)); got.Error != "" || got.SHA256 != checksum(data) {
		t.Fatalf("finalize: %+v", got)
	}
	if got := send("finalize", 0, nil, checksum(data)); got.Error != "" {
		t.Fatalf("finalize replay: %+v", got)
	}
	if got := send("download", 0, nil, ""); got.Error != "" || string(got.Data) != string(data) || got.SHA256 != checksum(data) {
		t.Fatalf("download: %+v", got)
	}
	if got := send("upload", 0, data, checksum(data)); got.Error == "" {
		t.Fatal("immutable object overwritten")
	}
}
func TestRejectsTraversalAndWrongWorkload(t *testing.T) {
	a, err := New(privateDataDir(t), "files", 1<<30)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	for _, id := range []string{"../../secret", "/etc/passwd", "x", "a/b"} {
		got := a.Handle(guestproto.Request{Version: 1, RequestID: state.Random(), Operation: "download", ObjectID: id})
		if got.Error != "INVALID_REQUEST" {
			t.Fatalf("id %q accepted", id)
		}
	}
	got := a.Handle(guestproto.Request{Version: 1, RequestID: state.Random(), Operation: "run", ObjectID: state.Random(), InputID: state.Random(), Preset: "mp4-720p"})
	if got.Error == "" {
		t.Fatal("files guest ran video")
	}
}
func TestRestartInterruptsInFlightTask(t *testing.T) {
	dir := privateDataDir(t)
	id := state.Random()
	data, _ := json.Marshal(task{State: "running", InputID: state.Random(), Preset: "mp4-720p", PromptHash: promptDigest(guestproto.Request{})})
	if err := os.WriteFile(filepath.Join(dir, id+".task.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	a, err := New(dir, "video", 8<<30)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	got := a.Handle(guestproto.Request{Version: 1, RequestID: state.Random(), Operation: "result", ObjectID: id})
	if got.State != "interrupted" {
		t.Fatalf("crashed work reported %+v", got)
	}
}
func TestSymlinkCannotEscapeGuestData(t *testing.T) {
	base := privateDataDir(t)
	dir := filepath.Join(base, "objects")
	a, err := New(dir, "files", 1<<30)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	secret := filepath.Join(base, "secret")
	if err = os.WriteFile(secret, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	id := state.Random()
	if err = os.Symlink(secret, filepath.Join(dir, id+".blob")); err != nil {
		t.Fatal(err)
	}
	got := a.Handle(guestproto.Request{Version: 1, RequestID: state.Random(), Operation: "download", ObjectID: id})
	if got.Error == "" {
		t.Fatal("symlink escaped root")
	}
}
