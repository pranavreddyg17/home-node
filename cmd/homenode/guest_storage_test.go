package main

import (
	"bytes"
	"context"
	"errors"
	"github.com/pranavreddyg17/home-node/internal/install"
	"testing"
)

func TestGuestStorageCommandRefusesInvalidInputsWithoutOutput(t *testing.T) {
	for _, tc := range []struct {
		mode string
		args []string
	}{
		{"activate", nil}, {"plan", nil}, {"prepare", []string{"--first-uid", "1", "--last-uid", "2"}},
		{"plan", []string{"--first-uid", "200000", "--last-uid", "199999"}},
		{"plan", []string{"--first-uid", "200000", "--last-uid", "265536"}},
		{"plan", []string{"--first-uid", "200000", "--last-uid", "2147483648"}},
		{"check", []string{"--first-uid", "200000"}}, {"check", []string{"--journal-dir", ""}}, {"check", []string{"extra"}},
	} {
		var out bytes.Buffer
		if err := runGuestStorage(context.Background(), tc.args, &out, tc.mode); !errors.Is(err, install.ErrPlan) || out.Len() != 0 {
			t.Fatal("invalid storage command admitted", tc, err, out.String())
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out bytes.Buffer
	if err := runGuestStorage(ctx, []string{"--first-uid", "200000", "--last-uid", "200001"}, &out, "prepare"); !errors.Is(err, context.Canceled) || out.Len() != 0 {
		t.Fatal("cancelled storage command admitted", err, out.String())
	}
}
