//go:build linux

package install

import (
	"context"
	"errors"
	"testing"
)

func TestGuestStorageConfigurationExcludedRejectsMissingAuthorityBeforeObservation(t *testing.T) {
	var e *Engine
	observed := false
	observer := func(context.Context) error { observed = true; return nil }
	if err := e.publishGuestStorageConfigurationExcludedLocked(context.Background(), observer, observer); !errors.Is(err, ErrPlan) || observed {
		t.Fatal("missing installation authority observed host", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := e.publishGuestStorageConfigurationExcludedLocked(ctx, observer, observer); !errors.Is(err, context.Canceled) || observed {
		t.Fatal("cancelled publication observed host", err)
	}
}
