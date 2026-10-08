package main

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

func allocatorPreparationArgs() []string {
	return []string{"--first-uid", "2000000000", "--last-uid", "2000000001", "--uid-min", "1000", "--uid-max", "60000", "--sys-uid-min", "100", "--sys-uid-max", "999", "--sub-uid-min", "100000", "--sub-uid-max", "600100000"}
}

func TestAllocatorPreparationRejectsAmbiguousInputsBeforeHostEffects(t *testing.T) {
	valid := allocatorPreparationArgs()
	for _, args := range [][]string{nil, valid[:14], append(append([]string(nil), valid...), "extra"), append(append([]string(nil), valid...), "--apply"), append(append([]string(nil), valid...), "--journal-dir", ""), append(append([]string(nil), valid...), "--uid-min=1000")} {
		var out bytes.Buffer
		if err := runGuestAllocationPrepare(context.Background(), args, &out); err == nil || out.Len() != 0 {
			t.Fatal("ambiguous allocator command admitted", err)
		}
	}
	for _, value := range []string{"0", "-1", "+1000", "01000", "0x3e8", "4294967296"} {
		args := allocatorPreparationArgs()
		args[5] = value
		var out bytes.Buffer
		if err := runGuestAllocationPrepare(context.Background(), args, &out); err == nil || out.Len() != 0 {
			t.Fatal("noncanonical allocator boundary admitted", value, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out bytes.Buffer
	if err := runGuestAllocationPrepare(ctx, valid, &out); !errors.Is(err, context.Canceled) || out.Len() != 0 {
		t.Fatal("canceled allocator command supplied status", err)
	}
}

func TestAllocatorApplicationRejectsOverridesBeforeHostEffects(t *testing.T) {
	for _, args := range [][]string{{"extra"}, {"--first-uid", "2000000000"}, {"--sys-uid-min", "100"}, {"--journal-dir", ""}, {"--journal-dir"}} {
		var out bytes.Buffer
		if err := runGuestAllocationApply(context.Background(), args, &out); err == nil || out.Len() != 0 {
			t.Fatal("allocator application accepted overrides or invalid input", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out bytes.Buffer
	if err := runGuestAllocationApply(ctx, nil, &out); !errors.Is(err, context.Canceled) || out.Len() != 0 {
		t.Fatal("canceled allocator application emitted status", err)
	}
}
