package runtimeclient

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestSnapshotReceiverRefusesInvalidStreamsAndPreservesExistingFile(t *testing.T) {
	for _, scenario := range []string{"invalid-database", "truncated", "extra-bytes", "wrong-type", "unknown-length", "oversized", "existing"} {
		t.Run(scenario, func(t *testing.T) {
			path := t.TempDir()
			if err := os.Chmod(path, 0700); err != nil {
				t.Fatal(err)
			}
			root, err := os.OpenRoot(path)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			if scenario == "existing" {
				if err = root.WriteFile("snapshot.db", []byte("preserved"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			device, job, token := state.Random(), state.Random(), state.Random()
			client := &MaintenanceAppsClient{inspect: func(context.Context, string) (state.MaintenanceJob, error) {
				return state.MaintenanceJob{ID: job, Device: device}, nil
			}, client: &http.Client{Transport: maintenanceRoundTrip(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path != "/v1/maintenance/snapshot" {
					t.Fatal("wrong operation")
				}
				response := &http.Response{StatusCode: 200, ContentLength: 6, Header: http.Header{"Content-Type": []string{"application/vnd.homenode.recovery-snapshot"}}, Body: io.NopCloser(strings.NewReader("broken"))}
				switch scenario {
				case "truncated":
					response.ContentLength = 7
				case "extra-bytes":
					response.ContentLength = 5
				case "wrong-type":
					response.Header.Set("Content-Type", "application/octet-stream")
				case "unknown-length":
					response.ContentLength = -1
				case "oversized":
					response.ContentLength = (256 << 20) + 1
				}
				return response, nil
			})}}
			if err = client.StageManagementSnapshot(context.Background(), token, device, root); err == nil {
				t.Fatal("invalid snapshot accepted")
			}
			data, err := root.ReadFile("snapshot.db")
			if scenario == "existing" {
				if err != nil || string(data) != "preserved" {
					t.Fatal("existing file changed", err)
				}
			} else if !os.IsNotExist(err) {
				t.Fatal("failed snapshot retained", err)
			}
		})
	}
}
