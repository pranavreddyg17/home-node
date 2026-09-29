package workload

import (
	"context"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestCancelUploadIsScopedDurableAndPreservesQuota(t *testing.T) {
	s, backend, device := service(t)
	ctx := context.Background()
	startFiles(t, s, device)
	data := []byte("discard these bytes")
	upload, err := s.CreateTransfer(ctx, device, "partial.txt", int64(len(data)), sum(data))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Upload(ctx, device, upload.ID, 0, data, sum(data)); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CancelTransfer(ctx, state.Random(), upload.ID); err == nil {
		t.Fatal("another device cancelled upload")
	}
	s.Backend = nil
	cancelled, err := s.CancelTransfer(ctx, device, upload.ID)
	if err != nil || cancelled.State != "cancelling" {
		t.Fatal(cancelled, err)
	}
	var reserved int64
	if err = s.Store.DB.QueryRow("SELECT coalesce(sum(size),0) FROM transfers WHERE state IN('uploading','verifying','cancelling')").Scan(&reserved); err != nil || reserved != int64(len(data)) {
		t.Fatal("reservation lost", reserved, err)
	}
	if _, err = s.Finalize(ctx, device, upload.ID); err == nil {
		t.Fatal("cancelled upload finalized")
	}
	if _, err = s.Upload(ctx, device, upload.ID, 0, data, sum(data)); err == nil {
		t.Fatal("cancelled upload resumed")
	}
	if _, err = s.CancelTransfer(ctx, device, upload.ID); err != nil {
		t.Fatal("cancel replay", err)
	}
	s.Backend = backend
	if err = s.expireTransfer(ctx); err != nil {
		t.Fatal(err)
	}
	cancelled, err = s.Transfer(ctx, device, upload.ID)
	if err != nil || cancelled.State != "cancelled" {
		t.Fatal(cancelled, err)
	}
}

func TestExpiredUploadRetainsReservationUntilGuestDeletion(t *testing.T) {
	s, backend, device := service(t)
	ctx := context.Background()
	startFiles(t, s, device)
	data := []byte("unfinished private upload")
	upload, err := s.CreateTransfer(ctx, device, "unfinished.txt", int64(len(data)), sum(data))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Upload(ctx, device, upload.ID, 0, data, sum(data)); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Store.DB.Exec("UPDATE transfers SET expires_at=0 WHERE id=?", upload.ID); err != nil {
		t.Fatal(err)
	}
	s.Backend = nil
	if err = s.expireTransfer(ctx); err == nil {
		t.Fatal("unavailable guest must keep reservation")
	}
	still, err := s.Transfer(ctx, device, upload.ID)
	if err != nil || still.State != "uploading" {
		t.Fatal(still, err)
	}
	s.Backend = backend
	if err = s.expireTransfer(ctx); err != nil {
		t.Fatal(err)
	}
	still, err = s.Transfer(ctx, device, upload.ID)
	if err != nil || still.State != "expired" {
		t.Fatal(still, err)
	}
	// Finalization would succeed if the old partial bytes had survived cleanup.
	response := backend.agent.Handle(guestproto.Request{Version: 1, RequestID: upload.ID, Operation: "finalize", ObjectID: upload.ID, Size: upload.Size, SHA256: upload.SHA256})
	if response.Error == "" {
		t.Fatal("expired content still exists")
	}
	if _, err = s.Finalize(ctx, device, upload.ID); err == nil {
		t.Fatal("expired upload became a file")
	}
	if err = s.expireTransfer(ctx); err != nil {
		t.Fatal("cleanup replay", err)
	}
}

func TestTrashExpirationReclaimsBytesOnlyAfterGuestDeletion(t *testing.T) {
	s, backend, device := service(t)
	ctx := context.Background()
	startFiles(t, s, device)
	data := []byte("retained until durable trash deletion")
	upload, err := s.CreateTransfer(ctx, device, "trash.txt", int64(len(data)), sum(data))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Upload(ctx, device, upload.ID, 0, data, sum(data)); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Finalize(ctx, device, upload.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.ChangeFile(ctx, device, upload.ID, "trash", ""); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Store.DB.Exec("UPDATE files SET trash_until=0 WHERE id=?", upload.ID); err != nil {
		t.Fatal(err)
	}
	s.Backend = nil
	if err = s.expireTrash(ctx); err == nil {
		t.Fatal("offline deletion falsely completed")
	}
	var bytes int64
	if err = s.Store.DB.QueryRow("SELECT sum(size) FROM files").Scan(&bytes); err != nil || bytes != int64(len(data)) {
		t.Fatal("quota released early", bytes, err)
	}
	if err = s.ChangeFile(ctx, device, upload.ID, "restore", ""); err == nil {
		t.Fatal("expired trash restored")
	}
	s.Backend = backend
	if err = s.expireTrash(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = s.File(ctx, upload.ID); err == nil {
		t.Fatal("purged file remained visible")
	}
	files, err := s.Files(ctx)
	if err != nil || len(files) != 0 {
		t.Fatal(files, err)
	}
	if err = s.Store.DB.QueryRow("SELECT sum(size) FROM files").Scan(&bytes); err != nil || bytes != 0 {
		t.Fatal("purge quota", bytes, err)
	}
	if response := backend.agent.Handle(guestproto.Request{Version: 1, RequestID: state.Random(), Operation: "stat", ObjectID: upload.ID}); response.Error == "" {
		t.Fatal("guest retained expired object")
	}
	if err = s.ChangeFile(ctx, device, upload.ID, "restore", ""); err == nil {
		t.Fatal("purged tombstone restored")
	}
	if _, err = s.Finalize(ctx, device, upload.ID); err == nil {
		t.Fatal("ready transfer resurrected purged file")
	}
	if err = s.expireTrash(ctx); err != nil {
		t.Fatal("purge replay", err)
	}
}

func TestTrashExpirationProtectsActiveJobInput(t *testing.T) {
	s, _, device, input := jobService(t)
	ctx := context.Background()
	job, err := s.CreateJob(ctx, device, state.Random(), input.ID, "mp4-720p", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ChangeFile(ctx, device, input.ID, "trash", ""); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Store.DB.Exec("UPDATE files SET trash_until=0 WHERE id=?", input.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.expireTrash(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = s.File(ctx, input.ID); err != nil {
		t.Fatal("active input deleted", err)
	}
	if err = s.CancelJob(ctx, device, job.ID, job.AttemptID); err != nil {
		t.Fatal(err)
	}
	if err = s.processJob(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.expireTrash(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = s.File(ctx, input.ID); err == nil {
		t.Fatal("terminal input held forever")
	}
}
