package workload

import (
	"context"
	"errors"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestFilesStorageTracksReservationsTrashAndCleanup(t *testing.T) {
	s, _, device := service(t)
	ctx := context.Background()
	startFiles(t, s, device)
	read := func() FilesStorage {
		t.Helper()
		usage, err := s.FilesStorage(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if usage.ChargedBytes != usage.StoredBytes+usage.UploadReservedBytes+usage.JobReservedBytes+usage.CleanupReservedBytes || usage.AvailableBytes != max(0, StorageQuota-usage.ChargedBytes) {
			t.Fatal("inconsistent storage snapshot", usage)
		}
		return usage
	}
	if usage := read(); usage.ChargedBytes != 0 || usage.AvailableBytes != StorageQuota {
		t.Fatal(usage)
	}
	data := []byte("file retained in trash")
	transfer, err := s.CreateTransfer(ctx, device, "storage.txt", int64(len(data)), sum(data))
	if err != nil {
		t.Fatal(err)
	}
	if usage := read(); usage.UploadReservedBytes != int64(len(data)) || usage.StoredBytes != 0 {
		t.Fatal(usage)
	}
	if _, err := s.Upload(ctx, device, transfer.ID, 0, data, sum(data)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Finalize(ctx, device, transfer.ID); err != nil {
		t.Fatal(err)
	}
	if usage := read(); usage.StoredBytes != int64(len(data)) || usage.UploadReservedBytes != 0 || usage.TrashBytes != 0 {
		t.Fatal(usage)
	}
	if _, err := s.CreateJob(ctx, device, state.Random(), transfer.ID, "mp4-720p", ""); err != nil {
		t.Fatal(err)
	}
	if usage := read(); usage.JobReservedBytes != MaxJobOutputBytes {
		t.Fatal(usage)
	}
	if err := s.ChangeFile(ctx, device, transfer.ID, "trash", ""); err != nil {
		t.Fatal(err)
	}
	if usage := read(); usage.TrashBytes != int64(len(data)) || usage.StoredBytes != int64(len(data)) {
		t.Fatal(usage)
	}
	if _, err := s.Store.DB.Exec("INSERT INTO orphan_objects VALUES(?,'files',1)", state.Random()); err != nil {
		t.Fatal(err)
	}
	if usage := read(); usage.CleanupReservedBytes != MaxJobOutputBytes {
		t.Fatal(usage)
	}
	partial, err := s.CreateTransfer(ctx, device, "partial.txt", 7, sum(nil))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CancelTransfer(ctx, device, partial.ID); err != nil {
		t.Fatal(err)
	}
	if usage := read(); usage.UploadReservedBytes != 7 {
		t.Fatal("cancel released quota early", usage)
	}
	if err := s.expireTransfer(ctx); err != nil {
		t.Fatal(err)
	}
	if usage := read(); usage.UploadReservedBytes != 0 {
		t.Fatal(usage)
	}
}

func TestJobAdmissionReportsTheSameCapacityAsFilesStorage(t *testing.T) {
	s, _, device := service(t)
	ctx := context.Background()
	startFiles(t, s, device)
	transfer, err := s.CreateTransfer(ctx, device, "empty.mp4", 0, sum(nil))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Finalize(ctx, device, transfer.ID); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if _, err := s.CreateJob(ctx, device, state.Random(), transfer.ID, "mp4-720p", ""); err != nil {
			t.Fatal(err)
		}
	}
	usage, err := s.FilesStorage(ctx)
	if err != nil || usage.AvailableBytes != 0 || usage.ChargedBytes != StorageQuota {
		t.Fatal(usage, err)
	}
	if _, err := s.CreateJob(ctx, device, state.Random(), transfer.ID, "mp4-720p", ""); !errors.Is(err, ErrCapacity) {
		t.Fatal("full quota was not reported as capacity refusal", err)
	}
	if _, err := s.CreateTransfer(ctx, device, "refused.txt", 1, sum(nil)); !errors.Is(err, ErrCapacity) {
		t.Fatal("displayed full quota did not govern uploads", err)
	}
}
