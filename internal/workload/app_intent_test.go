package workload

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestInvalidAppIntentCannotReachRuntimeOrStarveValidIntent(t *testing.T) {
	for _, alter := range []func(string) string{
		func(s string) string { return s[:len(s)-1] + `,"action":"stop"}` },
		func(s string) string { return s[:len(s)-1] + `,"act\u0069on":"stop"}` },
		func(s string) string { return s[:len(s)-1] + `,"unknown":true}` },
		func(s string) string { return strings.Replace(s, `"revision":1`, `"revision":null`, 1) },
		func(s string) string { return strings.Replace(s, `"action":"start"`, `"action":"inspect"`, 1) },
		func(s string) string { return s[:len(s)-1] + `,"cooperativeStop":true}` },
	} {
		s, original, device := service(t)
		ctx := context.Background()
		bad, err := s.AppAction(ctx, device, state.Random(), "files", "start")
		if err != nil {
			t.Fatal(err)
		}
		good, err := s.AppAction(ctx, device, state.Random(), "files", "stop")
		if err != nil {
			t.Fatal(err)
		}
		payload := alter(string(bad.Result))
		if json.Valid([]byte(payload)) == false {
			t.Fatal("fixture is not JSON")
		}
		if _, err = s.Store.DB.Exec("UPDATE operations SET result=?,created_at=0 WHERE id=?", payload, bad.ID); err != nil {
			t.Fatal(err)
		}
		backend := &appShutdownBackend{testBackend: original}
		s.Backend = backend
		if err = s.processApp(ctx); !errors.Is(err, ErrConflict) {
			t.Fatal("invalid intent accepted", err)
		}
		if len(backend.requests) != 0 {
			t.Fatal("invalid intent reached runtime", backend.requests)
		}
		badStatus, err := s.Operation(ctx, device, bad.ID)
		if err != nil || badStatus.State != "requires-action" || string(badStatus.Result) != payload {
			t.Fatal("invalid intent not retained", badStatus, err)
		}
		if err = s.processApp(ctx); err != nil {
			t.Fatal("invalid intent starved valid work", err)
		}
		goodStatus, err := s.Operation(ctx, device, good.ID)
		if err != nil || goodStatus.State != "succeeded" || len(backend.requests) != 1 {
			t.Fatal("valid intent failed", goodStatus, err)
		}
	}
}
