package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/install"
)

func TestGuestStorageImageMigrationCommandRequiresIndependentTrust(t *testing.T) {
	key := strings.Repeat("a", 64)
	for _, args := range [][]string{
		nil,
		{"--publisher-key", key},
		{"--publisher-key", "invalid", "--minimum-catalog-version", "1"},
		{"--publisher-key", key, "--minimum-catalog-version", "0"},
		{"--publisher-key", key, "--minimum-catalog-version", "1", "--journal-dir", ""},
		{"--publisher-key", key, "--minimum-catalog-version", "1", "unexpected"},
	} {
		var out bytes.Buffer
		if err := runGuestStorageMigrateImages(context.Background(), args, &out); !errors.Is(err, install.ErrPlan) || out.Len() != 0 {
			t.Fatal("invalid migration command admitted", args, err, out.String())
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out bytes.Buffer
	if err := runGuestStorageMigrateImages(ctx, []string{"--publisher-key", key, "--minimum-catalog-version", "1"}, &out); !errors.Is(err, context.Canceled) || out.Len() != 0 {
		t.Fatal("cancelled migration command admitted", err, out.String())
	}
}
