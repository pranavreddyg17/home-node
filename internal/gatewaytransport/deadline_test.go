package gatewaytransport

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type deadlineFixture struct {
	*httptest.ResponseRecorder
	err       error
	deadlines int
	flushes   int
}

func (w *deadlineFixture) SetWriteDeadline(deadline time.Time) error {
	w.deadlines++
	if remaining := time.Until(deadline); remaining <= 0 || remaining > 15*time.Second {
		return errors.New("unexpected deadline")
	}
	return w.err
}
func (w *deadlineFixture) FlushError() error { w.flushes++; return nil }

func TestStreamWriterRefreshesAndPropagatesDeadlines(t *testing.T) {
	fixture := &deadlineFixture{ResponseRecorder: httptest.NewRecorder()}
	w := &deadlineWriter{ResponseWriter: fixture}
	if _, err := w.Write([]byte("chunk")); err != nil {
		t.Fatal(err)
	}
	if err := http.NewResponseController(w).Flush(); err != nil {
		t.Fatal(err)
	}
	if fixture.deadlines != 2 || fixture.flushes != 1 || fixture.Body.String() != "chunk" {
		t.Fatal("stream did not refresh before write and flush")
	}
	failure := errors.New("closed stream")
	fixture.err = failure
	if n, err := w.Write([]byte("forbidden")); n != 0 || !errors.Is(err, failure) {
		t.Fatal("deadline error discarded", n, err)
	}
	if err := http.NewResponseController(w).Flush(); !errors.Is(err, failure) {
		t.Fatal("flush deadline error discarded", err)
	}
	if fixture.flushes != 1 || fixture.Body.String() != "chunk" {
		t.Fatal("output continued after deadline failure")
	}
}
