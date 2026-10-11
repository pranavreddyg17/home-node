package main

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

func TestConfigurationRemovalRejectsInputsBeforeHostEffects(t *testing.T) {
	for _, mode := range []string{"plan", "prepare", "apply", "retire"} {
		for _, args := range [][]string{{"--journal-dir", ""}, {"extra"}, {"--purge"}, {"--journal-dir"}} {
			var out bytes.Buffer
			if err := runConfigurationRemoval(context.Background(), args, mode, &out); err == nil || out.Len() != 0 {
				t.Fatal(mode, args, err)
			}
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		var out bytes.Buffer
		if err := runConfigurationRemoval(ctx, nil, mode, &out); !errors.Is(err, context.Canceled) || out.Len() != 0 {
			t.Fatal(mode, err)
		}
	}
	var out bytes.Buffer
	if err := runConfigurationRemoval(context.Background(), nil, "purge", &out); err == nil || out.Len() != 0 {
		t.Fatal("unknown destructive phase admitted", err)
	}
}
