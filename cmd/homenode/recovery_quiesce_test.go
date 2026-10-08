package main

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/install"
)

func TestRecoveryQuiesceRejectsInvalidInputsBeforeHostEffects(t *testing.T) {
	for _, args := range [][]string{{"--journal-dir", ""}, {"extra"}, {"--activate"}, {"--journal-dir"}} {
		var out bytes.Buffer
		if err := runRecoveryQuiesce(context.Background(), args, &out); err == nil || out.Len() != 0 {
			t.Fatal("invalid quiescence command admitted", args, err, out.String())
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out bytes.Buffer
	if err := runRecoveryQuiesce(ctx, nil, &out); !errors.Is(err, context.Canceled) || out.Len() != 0 {
		t.Fatal("cancelled command admitted", err, out.String())
	}
	if err := runRecoveryQuiesce(ctx, []string{"extra"}, &out); !errors.Is(err, install.ErrPlan) {
		t.Fatal("invalid arguments ignored", err)
	}
}
