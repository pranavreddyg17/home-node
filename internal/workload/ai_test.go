package workload

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"github.com/pranavreddyg17/home-node/internal/state"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

type aiBackend struct {
	base    *testBackend
	results map[string]guestproto.Response
	calls   int
}

func (b *aiBackend) Apply(ctx context.Context, r supervisor.Request) (supervisor.Instance, error) {
	return b.base.Apply(ctx, r)
}
func (b *aiBackend) Call(ctx context.Context, id string, r guestproto.Request) (guestproto.Response, error) {
	switch r.Operation {
	case "generate":
		if r.Prompt != "" || len(r.Messages) > 0 || !guestproto.ValidID(r.InputID) {
			return guestproto.Response{}, ErrInvalid
		}
		b.calls++
		b.results[r.ObjectID] = guestproto.Response{State: "succeeded", Text: "local answer fixture"}
		return guestproto.Response{Version: 1, RequestID: r.RequestID, State: "running"}, nil
	case "result":
		response, ok := b.results[r.ObjectID]
		if !ok {
			response.Error = "NOT_FOUND"
		}
		response.Version = 1
		response.RequestID = r.RequestID
		return response, nil
	case "cancel":
		b.results[r.ObjectID] = guestproto.Response{State: "cancelled"}
		return guestproto.Response{Version: 1, RequestID: r.RequestID, State: "cancelled"}, nil
	case "delete":
		delete(b.results, r.ObjectID)
	}
	return b.base.Call(ctx, id, r)
}
func aiService(t *testing.T) (*Service, *aiBackend, string, Conversation) {
	t.Helper()
	s, base, device := service(t)
	backend := &aiBackend{base: base, results: map[string]guestproto.Response{}}
	s.Backend = backend
	if _, err := s.Store.DB.Exec("UPDATE devices SET capabilities='[\"admin\",\"ai\"]'"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppAction(context.Background(), device, state.Random(), "ai", "start"); err != nil {
		t.Fatal(err)
	}
	if err := s.processApp(context.Background()); err != nil {
		t.Fatal(err)
	}
	conversation, err := s.CreateConversation(context.Background(), "Test conversation")
	if err != nil {
		t.Fatal(err)
	}
	return s, backend, device, conversation
}
func TestAIContentStaysOnGuestAndCanBeDeleted(t *testing.T) {
	s, b, device, c := aiService(t)
	ctx := context.Background()
	prompt := "private prompt never persisted in management"
	key := state.Random()
	g, err := s.CreateGeneration(ctx, device, key, c.ID, prompt)
	if err != nil {
		t.Fatal(err)
	}
	if g.State != "queued" {
		t.Fatal(g.State)
	}
	if err = s.processGeneration(ctx); err != nil {
		t.Fatal(err)
	}
	result, err := s.Generation(ctx, g.ID)
	if err != nil || result.Prompt != prompt || result.Output != "local answer fixture" || result.State != "succeeded" {
		t.Fatalf("generation %+v %v", result, err)
	}
	if _, err = s.CreateGeneration(ctx, device, key, c.ID, prompt); err != nil {
		t.Fatal(err)
	}
	if b.calls != 1 {
		t.Fatal("generation replay executed")
	}
	var reference, output, operationResult string
	if err = s.Store.DB.QueryRow("SELECT prompt,output FROM generations WHERE id=?", g.ID).Scan(&reference, &output); err != nil {
		t.Fatal(err)
	}
	if err = s.Store.DB.QueryRow("SELECT result FROM operations WHERE id=?", g.OperationID).Scan(&operationResult); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(reference, prompt) || output != "" || strings.Contains(operationResult, prompt) {
		t.Fatal("chat content escaped into management DB")
	}
	var ref objectReference
	if err = json.Unmarshal([]byte(reference), &ref); err != nil || !guestproto.ValidID(ref.ID) {
		t.Fatal("missing guest content reference")
	}
	history, err := s.History(ctx, c.ID)
	if err != nil || len(history) != 1 {
		t.Fatal(history, err)
	}
	if err = s.DeleteConversation(ctx, device, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.generation(ctx, g.ID); err == nil {
		t.Fatal("deleted history remained accessible")
	}
	response := b.base.agent.Handle(guestproto.Request{Version: 1, RequestID: state.Random(), Operation: "download", ObjectID: ref.ID})
	if response.Error != "NOT_FOUND" {
		t.Fatal("prompt blob survived deletion")
	}
}
func TestCancelQueuedGenerationDoesNotStart(t *testing.T) {
	s, b, device, c := aiService(t)
	ctx := context.Background()
	g, err := s.CreateGeneration(ctx, device, state.Random(), c.ID, "cancel me")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.CancelGeneration(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.processGeneration(ctx); err != nil {
		t.Fatal(err)
	}
	if b.calls != 0 {
		t.Fatal("cancelled generation ran")
	}
	result, err := s.Generation(ctx, g.ID)
	if err != nil || result.State != "cancelled" {
		t.Fatal(result, err)
	}
	if _, err = s.CreateGeneration(ctx, device, state.Random(), c.ID, "new request after cancellation"); err != nil {
		t.Fatal(err)
	}
}
func TestConversationBoundsAndActiveDeletion(t *testing.T) {
	s, _, device, c := aiService(t)
	ctx := context.Background()
	if _, err := s.CreateGeneration(ctx, device, state.Random(), c.ID, strings.Repeat("x", 2049)); err == nil {
		t.Fatal("unbounded prompt accepted")
	}
	if _, err := s.CreateGeneration(ctx, device, state.Random(), c.ID, "hello"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteConversation(ctx, device, c.ID); err == nil {
		t.Fatal("active conversation deleted")
	}
	if _, err := s.CreateGeneration(ctx, device, state.Random(), c.ID, "overlapping prompt"); err == nil {
		t.Fatal("concurrent conversation context accepted")
	}
}
