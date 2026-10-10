package workload

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/guest"
	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestCapacityRefusalPreservesUploadProgressAndAllowsResume(t *testing.T) {
	s, backend, device := service(t)
	ctx := context.Background()
	startFiles(t, s, device)
	dir := filepath.Join(t.TempDir(), "guest-data")
	agent, err := guest.New(dir, "files", 1<<30)
	if err != nil {
		t.Fatal(err)
	}
	defer agent.Close()
	backend.agent = agent
	blocker := filepath.Join(dir, state.Random()+".blob")
	file, err := os.OpenFile(blocker, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	// Sparse private fixture object consumes the guest's logical accounting;
	// this does not fill or alter the owner's filesystem.
	err = file.Truncate((1 << 30) - (64 << 20) - 4)
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		t.Fatal(err, closeErr)
	}
	content := []byte("retry after capacity is freed")
	transfer, err := s.CreateTransfer(ctx, device, "resume.txt", int64(len(content)), sum(content))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Upload(ctx, device, transfer.ID, 0, content, sum(content)); !errors.Is(err, ErrCapacity) {
		t.Fatal("capacity refusal classification lost", err)
	}
	current, err := s.Transfer(ctx, device, transfer.ID)
	if err != nil || current.Offset != 0 || current.State != "uploading" {
		t.Fatal("refusal changed upload progress", current, err)
	}
	if err := os.Remove(blocker); err != nil {
		t.Fatal(err)
	}
	resumed, err := s.Upload(ctx, device, transfer.ID, 0, content, sum(content))
	if err != nil || resumed.Offset != int64(len(content)) {
		t.Fatal("capacity recovery failed", resumed, err)
	}
	completed, err := s.Finalize(ctx, device, transfer.ID)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := s.Download(ctx, completed.ID, 0)
	if err != nil || string(actual) != string(content) {
		t.Fatal("resumed content changed", err)
	}
}
