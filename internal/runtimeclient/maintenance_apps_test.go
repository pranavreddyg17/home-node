package runtimeclient

import (
	"context"
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
