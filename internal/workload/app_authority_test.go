package workload

import (
	"context"
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestQueuedAppAuthorityClockBounds(t *testing.T) {
	for _, tc := range []struct {
		created int64
		now     int64
		allowed bool
	}{
		{1000, 1000, true},
		{700, 1000, true},
		{699, 1000, false},
		{1001, 1000, false},
		{0, 1000, false},
		{-1, 1000, false},
		{math.MinInt64, math.MaxInt64, false},
		{math.MaxInt64, 1000, false},
		{1, math.MaxInt64, false},
		{1, -1, false},
	} {
		if got := appAuthorityCurrent(tc.created, tc.now); got != tc.allowed {
			t.Fatalf("created=%d now=%d allowed=%v", tc.created, tc.now, got)
		}
	}
}

func TestInvalidQueuedAppTimestampsNeverReachRuntime(t *testing.T) {
	for name, created := range map[string]int64{
		"future":   time.Now().Unix() + 3600,
		"expired":  time.Now().Unix() - 3600,
		"zero":     0,
		"negative": -1,
		"overflow": math.MinInt64,
	} {
		t.Run(name, func(t *testing.T) {
			s, original, device := service(t)
			b := &uncertainStartBackend{testBackend: original}
			s.Backend = b
			ctx := context.Background()
			op, err := s.AppAction(ctx, device, state.Random(), "files", "start")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.Store.DB.Exec("UPDATE operations SET created_at=? WHERE id=?", created, op.ID); err != nil {
				t.Fatal(err)
			}
			if err = s.processApp(ctx); err != nil {
				t.Fatal(err)
			}
			if len(b.requests) != 0 {
				t.Fatal("invalid clock authority reached runtime", b.requests)
			}
			result, err := s.Operation(ctx, device, op.ID)
			if err != nil || result.State != "failed" {
				t.Fatal(result, err)
			}
			var payload struct {
				Code string `json:"code"`
			}
			if err = json.Unmarshal(result.Result, &payload); err != nil || payload.Code != "AUTHORIZATION_EXPIRED" {
				t.Fatal("invalid authority not expired", payload, err)
			}
		})
	}
}
