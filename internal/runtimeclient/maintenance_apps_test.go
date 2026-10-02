package runtimeclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestAppMaintenanceClientRefusesOwnershipBeforeRequest(t *testing.T) {
	token, device, id := state.Random(), state.Random(), state.Random()
	for _, scenario := range []string{"foreign-device", "invalid-job", "inspection-failure"} {
		t.Run(scenario, func(t *testing.T) {
			invoked := false
			client := &MaintenanceAppsClient{client: &http.Client{Transport: maintenanceRoundTrip(func(*http.Request) (*http.Response, error) {
				invoked = true
				return nil, errors.New("unexpected request")
			})}, inspect: func(context.Context, string) (state.MaintenanceJob, error) {
				job := state.MaintenanceJob{ID: id, Device: device}
				switch scenario {
				case "foreign-device":
					job.Device = state.Random()
				case "invalid-job":
					job.ID = "short"
				case "inspection-failure":
					return job, errors.New("missing ownership")
				}
				return job, nil
			}}
			if err := client.DrainMaintenance(context.Background(), token, device); !errors.Is(err, ErrMaintenance) || invoked {
				t.Fatal("invalid ownership reached controller", invoked, err)
			}
		})
	}
}

func TestAppMaintenanceClientRequiresExactBoundedAcknowledgement(t *testing.T) {
	token, device, id := state.Random(), state.Random(), state.Random()
	for _, reply := range []string{`{"version":1}`, `{"version":1,"version":1}`, `{"version":1,"token":"extra"}`, `{"version":null}`, `{"version":2}`, `{"version":1}{}`, `{"version":1}` + strings.Repeat(" ", 512), `{"version":1}` + string([]byte{255})} {
		t.Run(reply[:min(len(reply), 40)], func(t *testing.T) {
			client := &MaintenanceAppsClient{client: &http.Client{Transport: maintenanceRoundTrip(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path != "/v1/maintenance/restore" || r.Method != "POST" {
					t.Fatal("unexpected controller operation")
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(reply)), Header: make(http.Header)}, nil
			})}, inspect: func(context.Context, string) (state.MaintenanceJob, error) {
				return state.MaintenanceJob{ID: id, Device: device}, nil
			}}
			err := client.RestoreMaintenanceApps(context.Background(), token, device)
			if (err == nil) != (reply == `{"version":1}`) {
				t.Fatal("unexpected acknowledgement admission", err)
			}
		})
	}
}

func TestPublicationAcknowledgementValidatesBeforeTransport(t *testing.T) {
	token, device, id := state.Random(), state.Random(), state.Random()
	calls := 0
	client := &MaintenanceAppsClient{client: &http.Client{Transport: maintenanceRoundTrip(func(request *http.Request) (*http.Response, error) {
		calls++
		if request.URL.Path != "/v1/maintenance/ack-publish" {
			t.Fatal("wrong publication endpoint")
		}
		data, err := io.ReadAll(request.Body)
		if err != nil || !strings.Contains(string(data), `"snapshotId":"`+state.Hash("snapshot")+`"`) {
			t.Fatal("missing bound snapshot", err)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"version":1}`))}, nil
	})}, inspect: func(context.Context, string) (state.MaintenanceJob, error) {
		return state.MaintenanceJob{ID: id, Device: device}, nil
	}}
	for _, invalid := range []string{"", "short", strings.Repeat("A", 64)} {
		if err := client.RecordPublication(context.Background(), token, device, invalid); err == nil || calls != 0 {
			t.Fatal("invalid snapshot reached transport")
		}
	}
	if err := client.RecordPublication(context.Background(), token, device, state.Hash("snapshot")); err != nil || calls != 1 {
		t.Fatal("valid acknowledgement failed", err)
	}
}

func TestOwnedMaintenanceClientBindsDispatchedIdentity(t *testing.T) {
	token, jobID, device := state.Random(), state.Random(), state.Random()
	client := NewOwnedMaintenanceApps("/tmp/owned-maintenance-fixture.sock", 1001, token, jobID, device)
	defer client.Close()
	calls := 0
	client.client.Transport = maintenanceRoundTrip(func(request *http.Request) (*http.Response, error) {
		calls++
		raw, err := io.ReadAll(request.Body)
		if err != nil || !strings.Contains(string(raw), `"jobId":"`+jobID+`"`) {
			t.Fatal("dispatch job binding lost", err)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"version":1}`))}, nil
	})
	if err := client.ConfirmStaging(context.Background(), state.Random(), device); err == nil || calls != 0 {
		t.Fatal("foreign token reached controller")
	}
	if err := client.ConfirmStaging(context.Background(), token, state.Random()); err == nil || calls != 0 {
		t.Fatal("foreign device reached controller")
	}
	if err := client.ConfirmStaging(context.Background(), token, device); err != nil || calls != 1 {
		t.Fatal("bound request refused", err)
	}
	for _, invalid := range []*MaintenanceAppsClient{
		NewOwnedMaintenanceApps("/tmp/owned-maintenance-fixture.sock", 0, token, jobID, device),
		NewActivatedOwnedMaintenanceApps("/tmp/owned-maintenance-fixture.sock", "short", jobID, device),
		NewActivatedOwnedMaintenanceApps("relative.sock", token, jobID, device),
	} {
		if invalid.client != nil {
			t.Fatal("invalid owned transport admitted")
		}
	}
}

func TestRuntimeRootCheckpointBindsOwnedJobAndToken(t *testing.T) {
	token, device, id, root := state.Random(), state.Random(), state.Random(), state.Random()
	calls := 0
	client := &MaintenanceAppsClient{client: &http.Client{Transport: maintenanceRoundTrip(func(request *http.Request) (*http.Response, error) {
		calls++
		if request.URL.Path != "/v1/maintenance/attach-root" || request.Method != "POST" {
			t.Fatal("wrong checkpoint operation")
		}
		data, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		var payload map[string]any
		if json.Unmarshal(data, &payload) != nil || len(payload) != 5 || payload["version"] != float64(1) || payload["token"] != token || payload["jobId"] != id || payload["deviceId"] != device || payload["rootToken"] != root {
			t.Fatal("checkpoint binding differs", string(data))
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"version":1}`)), Header: make(http.Header)}, nil
	})}, inspect: func(context.Context, string) (state.MaintenanceJob, error) {
		return state.MaintenanceJob{ID: id, Device: device}, nil
	}}
	if err := client.AttachRuntimeRoot(context.Background(), token, device, "short"); !errors.Is(err, ErrMaintenance) || calls != 0 {
		t.Fatal("invalid root reached transport", err, calls)
	}
	if err := client.AttachRuntimeRoot(context.Background(), token, device, root); err != nil || calls != 1 {
		t.Fatal("valid checkpoint refused", err, calls)
	}
}
