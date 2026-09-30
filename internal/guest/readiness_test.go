package guest

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestAIHealthWaitsForLoadedModel(t *testing.T) {
	for _, test := range []struct {
		status int
		body   string
		want   string
	}{
		{200, `{"status":"ok"}`, "ready"}, {200, `{"status":"ok","slots_idle":1,"slots_processing":0}`, "ready"},
		{503, `{"error":{"message":"Loading model"}}`, "starting"}, {200, `{"status":"loading model"}`, "starting"},
		{200, `{"status":"ok","status":"ok"}`, "starting"}, {200, `{"status":"ok"} {}`, "starting"},
		{200, strings.Repeat("x", 4097), "starting"}, {200, `{}`, "starting"}, {200, `{"status":"ok","tools":true}`, "starting"},
	} {
		t.Run(test.want+test.body[:min(12, len(test.body))], func(t *testing.T) {
			a, err := New(t.TempDir(), "ai", 1<<30)
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			a.readinessClient = &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
				if r.Method != "GET" || r.URL.String() != "http://127.0.0.1:8080/health" || r.Body != nil {
					t.Error("unexpected readiness authority", r.Method, r.URL)
				}
				if _, ok := r.Context().Deadline(); !ok {
					t.Error("missing deadline")
				}
				return &http.Response{StatusCode: test.status, Body: io.NopCloser(strings.NewReader(test.body))}, nil
			})}
			response := a.Handle(guestproto.Request{Version: 1, RequestID: state.Random(), Operation: "health"})
			if response.State != test.want || response.Error != "" {
				t.Fatal(response)
			}
		})
	}
}
func TestAIHealthFailureDoesNotClaimReady(t *testing.T) {
	a, err := New(t.TempDir(), "ai", 1<<30)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	a.readinessClient = &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) { return nil, errors.New("unavailable") })}
	response := a.Handle(guestproto.Request{Version: 1, RequestID: state.Random(), Operation: "health"})
	if response.State != "starting" {
		t.Fatal(response)
	}
	if a.client == a.readinessClient {
		t.Fatal("readiness shares generation client")
	}
}

func TestAIReadinessCannotFollowRedirects(t *testing.T) {
	a, err := New(t.TempDir(), "ai", 1<<30)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	calls := 0
	a.readinessClient.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": []string{"http://example.invalid/private"}}, Body: io.NopCloser(strings.NewReader("redirect")), Request: r}, nil
	})
	if a.modelReady() || calls != 1 {
		t.Fatal("readiness followed a redirect", calls)
	}
}
