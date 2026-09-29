package workload

import (
	"context"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
)

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
