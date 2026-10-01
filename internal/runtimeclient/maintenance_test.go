package runtimeclient

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestMaintenanceClientSocketReplay(t *testing.T) {
	directory, err := os.MkdirTemp("/tmp", "hn-m-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(directory)
	socket := filepath.Join(directory, "maintenance.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	job := strings.Repeat("j", 24)
	token := strings.Repeat("t", 24)
	var calls atomic.Int32
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]json.RawMessage
		if r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" || json.NewDecoder(r.Body).Decode(&body) != nil {
			http.Error(w, "bad request", 400)
			return
		}
		calls.Add(1)
		if r.URL.Path == "/v1/maintenance/begin" {
			var id string
			_ = json.Unmarshal(body["jobId"], &id)
			if id != job {
				http.Error(w, "bad job", 400)
				return
			}
			_, _ = w.Write([]byte(`{"version":1,"token":"` + token + `"}`))
		} else if r.URL.Path == "/v1/maintenance/end" {
			var id string
			_ = json.Unmarshal(body["token"], &id)
			if id != token {
				http.Error(w, "bad token", 400)
				return
			}
			_, _ = w.Write([]byte(`{"version":1}`))
		} else {
			http.NotFound(w, r)
		}
	})}
	done := make(chan struct{})
	go func() { defer close(done); _ = server.Serve(listener) }()
	defer func() { server.Close(); <-done }()
	client := NewMaintenance(socket)
	defer client.client.CloseIdleConnections()
	for i := 0; i < 2; i++ {
		got, err := client.BeginRuntimeMaintenanceForJob(context.Background(), job)
		if err != nil || got != token {
			t.Fatal(got, err)
		}
		if err = client.EndRuntimeMaintenance(context.Background(), token); err != nil {
			t.Fatal(err)
		}
	}
	// Stop the HTTP worker before inspecting its mutable fixture counter.
	server.Close()
	<-done
	if calls.Load() != 4 {
		t.Fatal(calls.Load())
	}
}

func TestMaintenanceAcknowledgementsRejectAmbiguity(t *testing.T) {
	token := strings.Repeat("t", 24)
	valid := `{"version":1,"token":"` + token + `"}`
	if got, err := decodeMaintenanceReply([]byte(valid), true); err != nil || got != token {
		t.Fatal(got, err)
	}
	for _, data := range []string{`null`, `[]`, valid + `{}`, `{"version":1,"version":1,"token":"` + token + `"}`, `{"version":1,"token":"` + token + `","to\u006ben":"` + token + `"}`, `{"version":1,"token":null}`, `{"version":1,"token":"short"}`, `{"version":1,"token":"` + token + `","extra":1}`, `{"version":2,"token":"` + token + `"}`} {
		if _, err := decodeMaintenanceReply([]byte(data), true); err == nil {
			t.Fatal("accepted", data)
		}
	}
	if _, err := decodeMaintenanceReply([]byte(valid), false); err == nil {
		t.Fatal("release accepted acquisition token")
	}
	if _, err := decodeMaintenanceReply([]byte(`{"version":1}`), true); err == nil {
		t.Fatal("acquisition accepted missing token")
	}
}

type maintenanceRoundTrip func(*http.Request) (*http.Response, error)

func (f maintenanceRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestMaintenanceClientRejectsOversizedAndInvalidUTF8Replies(t *testing.T) {
	for _, body := range []string{strings.Repeat(" ", 513) + `{"version":1}`, `{"version":1}` + string([]byte{0xff})} {
		client := &MaintenanceClient{client: &http.Client{Transport: maintenanceRoundTrip(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
		})}}
		if err := client.EndRuntimeMaintenance(context.Background(), strings.Repeat("t", 24)); err == nil {
			t.Fatal("invalid reply accepted")
		}
	}
}
