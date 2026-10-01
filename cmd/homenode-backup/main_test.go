package main

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func validArguments() []string {
	return []string{"--controller-uid", "351", "--controller-gid", "351", "--repository-mount", "/mnt/registered-backup", "--repository-uuid", "drive-fixture", "--repository-id", strings.Repeat("a", 64), "--release", "0.1.0", "--catalog-version", "2", "--minimum-catalog-version", "1"}
}
func TestBackupServiceRequiresExplicitTrustedConfiguration(t *testing.T) {
	if _, err := parseOptions(validArguments()); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{}, append(validArguments(), "unexpected"), append(validArguments(), "--password", "secret"), append(validArguments(), "--controller-uid", "0"), append(validArguments(), "--socket", "relative"), append(validArguments(), "--minimum-catalog-version", "3"), append(validArguments(), "--release", "../binary"), append(validArguments(), "--socket", "/run/homenode-backup/apps.sock"),
	} {
		if _, err := parseOptions(args); err == nil {
			t.Fatal("unsafe configuration accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := run(ctx, validArguments()); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled startup consumed activation", err)
	}
}
