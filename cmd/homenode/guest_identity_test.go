package main

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

func TestGuestIdentityPreparationRejectsInputsBeforeHostEffects(t *testing.T) {
	for _, args := range [][]string{{"--journal-dir", ""}, {"extra"}, {"--apply"}, {"--journal-dir"}} {
		var out bytes.Buffer
		if err := runGuestIdentityPrepare(context.Background(), args, &out); err == nil || out.Len() != 0 {
			t.Fatal("invalid identity command admitted", args, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out bytes.Buffer
	if err := runGuestIdentityPrepare(ctx, nil, &out); !errors.Is(err, context.Canceled) || out.Len() != 0 {
		t.Fatal("cancelled identity command admitted", err)
	}
}
