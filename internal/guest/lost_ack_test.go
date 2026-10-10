package guest

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"github.com/pranavreddyg17/home-node/internal/state"
)

type lostAcknowledgmentStream struct {
	io.Reader
	err error
}

func (s lostAcknowledgmentStream) Write([]byte) (int, error) { return 0, s.err }

func TestCommittedUploadSurvivesLostAcknowledgmentAndAgentReopen(t *testing.T) {
	dir := privateDataDir(t)
	a, err := New(dir, "files", 1<<30)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	content := []byte("committed without acknowledgment; resumed suffix")
	id := state.Random()
	first := guestproto.Request{Version: 1, RequestID: state.Random(), Operation: "upload", ObjectID: id,
		Size: int64(len(content)), Data: content[:20], SHA256: checksum(content[:20])}
	var frame bytes.Buffer
	if err := guestproto.Write(&frame, first); err != nil {
		t.Fatal(err)
	}
	disconnected := errors.New("fixture response channel disconnected")
	if err := a.Serve(lostAcknowledgmentStream{Reader: &frame, err: disconnected}); !errors.Is(err, disconnected) {
		t.Fatal("lost response was not observed", err)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	a, err = New(dir, "files", 1<<30)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	first.RequestID = state.Random()
	if r := a.Handle(first); r.Error != "" || r.Offset != 20 {
		t.Fatal("committed chunk replay failed", r)
	}
	last := first
	last.RequestID, last.Offset, last.Data, last.SHA256 = state.Random(), 20, content[20:], checksum(content[20:])
	if r := a.Handle(last); r.Error != "" || r.Offset != int64(len(content)) {
		t.Fatal("resume failed", r)
	}
	final := guestproto.Request{Version: 1, RequestID: state.Random(), Operation: "finalize", ObjectID: id,
		Size: int64(len(content)), SHA256: checksum(content)}
	if r := a.Handle(final); r.Error != "" || r.Size != final.Size || r.SHA256 != final.SHA256 {
		t.Fatal("finalization after lost acknowledgment failed", r)
	}
	final.Operation, final.RequestID = "download", state.Random()
	if r := a.Handle(final); r.Error != "" || !bytes.Equal(r.Data, content) || r.SHA256 != checksum(content) {
		t.Fatal("resumed bytes changed", r)
	}
}
