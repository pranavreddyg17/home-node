//go:build linux

package backup

import (
	"context"
	"os"
	"testing"
)

func TestRepositoryDirectoryLeaseExcludesIndependentHandles(t *testing.T) {
	path := t.TempDir()
	if err := os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
	first, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if err = leaseRepositoryDirectory(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err = leaseRepositoryDirectory(context.Background(), second); err == nil {
		t.Fatal("overlapping repository lease admitted")
	}
	first.Close()
	if err = leaseRepositoryDirectory(context.Background(), second); err != nil {
		t.Fatal("closed owner retained lease", err)
	}
}
func TestRepositoryDirectoryLeaseRefusesPublicOrCancelledHandles(t *testing.T) {
	path := t.TempDir()
	if err := os.Chmod(path, 0755); err != nil {
		t.Fatal(err)
	}
	directory, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	if err = leaseRepositoryDirectory(context.Background(), directory); err == nil {
		t.Fatal("public repository directory accepted")
	}
	if err = os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = leaseRepositoryDirectory(ctx, directory); err != context.Canceled {
		t.Fatal("cancelled lease acquired", err)
	}
}
