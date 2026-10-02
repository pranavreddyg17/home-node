//go:build linux

package updates

import (
	"context"
	"errors"
	"testing"
)

func TestInspectionBoundaryUsesLiveMonotonicClockAndCancellation(t *testing.T) {
	first, err := InspectionLaunchBoundary(context.Background())
	if err != nil || first == 0 {
		t.Fatal("kernel boundary unavailable", first, err)
	}
	second, err := InspectionLaunchBoundary(context.Background())
	if err != nil || second < first {
		t.Fatal("kernel boundary regressed", first, second, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if boundary, err := InspectionLaunchBoundary(ctx); boundary != 0 || !errors.Is(err, context.Canceled) {
		t.Fatal("canceled boundary admitted", boundary, err)
	}
}
