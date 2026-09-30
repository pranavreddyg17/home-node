package guest

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"github.com/pranavreddyg17/home-node/internal/state"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestBoundedChatStreamAndPersistedOutput(t *testing.T) {
	a, err := New(privateDataDir(t), "ai", 1<<30)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	a.client = &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "http://127.0.0.1:8080/v1/chat/completions" {
			t.Errorf("unexpected destination %s", r.URL)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["max_tokens"] != float64(256) || body["tools"] != nil {
			t.Error("unexpected inference authority")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":null},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"hello\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"))}, nil
	})}
	id := state.Random()
	response := a.Handle(guestproto.Request{Version: 1, RequestID: state.Random(), Operation: "generate", ObjectID: id, Prompt: "hello"})
	if response.Error != "" {
		t.Fatal(response)
	}
	deadline := time.After(3 * time.Second)
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline:
			t.Fatal("generation never completed")
		case <-ticker.C:
			result := a.Handle(guestproto.Request{Version: 1, RequestID: state.Random(), Operation: "result", ObjectID: id})
			if result.State == "running" {
				continue
			}
			if result.State != "succeeded" || result.Text != "hello" {
				t.Fatalf("unexpected result %+v", result)
			}
			return
		}
	}
}
func TestAgentRejectsAdministrativeChatRoles(t *testing.T) {
	r := guestproto.Request{Version: 1, RequestID: state.Random(), Operation: "generate", ObjectID: state.Random(), Messages: []guestproto.Message{{Role: "tool", Content: "run shell"}}}
	if err := guestproto.Validate(r); err == nil {
		t.Fatal("tool-role protocol accepted")
	}
}

func TestGenerationRequiresApprovedFinishAndDoneMarker(t *testing.T) {
	content := `data: {"choices":[{"delta":{"content":"hello"},"finish_reason":null}]}` + "\n\n"
	finish := func(reason string) string {
		return `data: {"choices":[{"delta":{},"finish_reason":"` + reason + `"}]}` + "\n\n"
	}
	for _, c := range []struct {
		name, stream string
		want         bool
	}{
		{"stop", content + finish("stop") + "data: [DONE]\n\n", true},
		{"length", content + finish("length") + "data: [DONE]\n\n", true},
		{"missing done", content + finish("stop"), false},
		{"missing finish", content + "data: [DONE]\n\n", false},
		{"tool finish", content + finish("tool_calls") + "data: [DONE]\n\n", false},
		{"unknown finish", content + finish("unexpected") + "data: [DONE]\n\n", false},
		{"content after finish", content + finish("stop") + content + "data: [DONE]\n\n", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			a, err := New(privateDataDir(t), "ai", 1<<30)
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			a.client = &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(c.stream))}, nil
			})}
			text, err := a.generate(context.Background(), guestproto.Request{ObjectID: state.Random(), Prompt: "hello"})
			if c.want {
				if err != nil || text != "hello" {
					t.Fatal("valid stream refused", text, err)
				}
			} else if err == nil {
				t.Fatal("invalid stream accepted", text)
			}
		})
	}
}
