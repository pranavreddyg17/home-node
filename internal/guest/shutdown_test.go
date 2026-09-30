package guest

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestCloseJoinsWorkerAndPersistsInterruptedTask(t *testing.T) {
	dir := privateDataDir(t)
	a, err := New(dir, "ai", 1<<30)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	started := make(chan struct{})
	observed := make(chan error, 1)
	a.client = &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		close(started)
		<-r.Context().Done()
		observed <- a.root.WriteFile("worker-final-write", []byte("joined"), 0600)
		return nil, r.Context().Err()
	})}
	id := state.Random()
	response := a.Handle(guestproto.Request{Version: 1, RequestID: state.Random(), Operation: "generate", ObjectID: id, Prompt: "test interruption"})
	if response.Error != "" {
		t.Fatal(response)
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not start")
	}
	if err = a.Close(); err != nil {
		t.Fatal(err)
	}
	if err = <-observed; err != nil {
		t.Fatal("data root closed before worker completed", err)
	}
	if err = a.Close(); err != nil {
		t.Fatal("close not idempotent", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, id+".task.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved task
	if err = json.Unmarshal(data, &saved); err != nil || saved.State != "interrupted" {
		t.Fatal("interruption not durable", saved.State, err)
	}
	response = a.Handle(guestproto.Request{Version: 1, RequestID: state.Random(), Operation: "health"})
	if response.State == "ready" || response.Error != "WORKLOAD_UNAVAILABLE" {
		t.Fatal("closed agent accepts requests", response)
	}
	restored, err := New(dir, "ai", 1<<30)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	result := restored.Handle(guestproto.Request{Version: 1, RequestID: state.Random(), Operation: "result", ObjectID: id})
	if result.State != "interrupted" {
		t.Fatal("interruption lost on restart", result)
	}
}

func TestClosingAgentCannotAdvertiseLateReadiness(t *testing.T) {
	a, err := New(privateDataDir(t), "ai", 1<<30)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan guestproto.Response, 1)
	a.readinessClient = &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		close(started)
		<-release
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"status":"ok"}`))}, nil
	})}
	go func() {
		done <- a.Handle(guestproto.Request{Version: 1, RequestID: state.Random(), Operation: "health"})
	}()
	<-started
	if err = a.Close(); err != nil {
		t.Fatal(err)
	}
	close(release)
	result := <-done
	if result.State == "ready" || result.Error != "WORKLOAD_UNAVAILABLE" {
		t.Fatal("late readiness after close", result)
	}
}
