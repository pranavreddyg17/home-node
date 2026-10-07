package main

import (
	"bytes"
	"context"
	"testing"
)

func TestGuestUIDPlanRefusesMalformedArgumentsWithoutOutput(t *testing.T) {
	for _, args := range [][]string{{"--first-uid", "4294967296"}, {"--last-uid", "-1"}, {"--controller-uid", "4294967296"}, {"--transfer-uid", "2147483648"}, {"--backup-uid", "4294967296"}, {"unexpected"}, {"--unknown"}} {
		var out bytes.Buffer
		if err := runGuestUIDPlan(context.Background(), args, &out); err == nil || out.Len() != 0 {
			t.Fatal("invalid proposal emitted", args, err, out.String())
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out bytes.Buffer
	if err := runGuestUIDPlan(ctx, nil, &out); err == nil || out.Len() != 0 {
		t.Fatal("cancelled proposal emitted", err)
	}
}

func TestGuestUIDPreparationRequiresJournalWithoutIdentityOverrides(t *testing.T) {
	for _, args := range [][]string{nil, {"--owner-id", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, {"--journal-dir", "/missing-fixture", "--controller-uid", "998"}} {
		var out bytes.Buffer
		if err := runGuestUIDProposal(context.Background(), args, &out, true); err == nil || out.Len() != 0 {
			t.Fatal("invalid preparation emitted", args, err)
		}
	}
}
