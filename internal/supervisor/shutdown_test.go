package supervisor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestShutdownWaitsForObservedExit(t *testing.T) {
	probes, requests := 0, 0
	err := awaitShutdown(context.Background(), state.Random(), func(context.Context, string) (bool, error) { probes++; return probes < 4, nil }, func(context.Context, string) error { requests++; return nil }, time.Millisecond)
	if err != nil || probes != 4 || requests != 1 {
		t.Fatal("shutdown acknowledgement treated as exit", probes, requests, err)
	}
}

func TestShutdownDoesNotForceStopOnCancellationOrFailure(t *testing.T) {
	for _, scenario := range []string{"already-stopped", "probe-failure", "request-failure", "cancelled", "invalid-id"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			requests := 0
			sentinel := errors.New("fixture failure")
			id := state.Random()
			if scenario == "invalid-id" {
				id = "../foreign"
			}
			err := awaitShutdown(ctx, id, func(context.Context, string) (bool, error) {
				if scenario == "probe-failure" {
					return false, sentinel
				}
				return scenario != "already-stopped", nil
			}, func(context.Context, string) error {
				requests++
				if scenario == "request-failure" {
					return sentinel
				}
				cancel()
				return nil
			}, time.Millisecond)
			switch scenario {
			case "already-stopped":
				if err != nil || requests != 0 {
					t.Fatal(requests, err)
				}
			case "probe-failure":
				if !errors.Is(err, sentinel) || requests != 0 {
					t.Fatal(requests, err)
				}
			case "request-failure":
				if !errors.Is(err, sentinel) || requests != 1 {
					t.Fatal(requests, err)
				}
			case "cancelled":
				if !errors.Is(err, context.Canceled) || requests != 1 {
					t.Fatal(requests, err)
				}
			case "invalid-id":
				if !errors.Is(err, ErrPolicy) || requests != 0 {
					t.Fatal(requests, err)
				}
			}
		})
	}
}
