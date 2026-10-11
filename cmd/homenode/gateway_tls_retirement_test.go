package main

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

func TestGatewayTLSRetirementRejectsInputsBeforeHostEffects(t *testing.T) {
	for _, args := range [][]string{{"--journal-dir", ""}, {"extra"}, {"--purge"}, {"--journal-dir"}, {"--key", "/etc/shadow"}} {
		var out bytes.Buffer
		if err := runGatewayTLSRetirement(context.Background(), args, &out); err == nil || out.Len() != 0 {
			t.Fatal(args, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out bytes.Buffer
	if err := runGatewayTLSRetirement(ctx, nil, &out); !errors.Is(err, context.Canceled) || out.Len() != 0 {
		t.Fatal(err)
	}
}
