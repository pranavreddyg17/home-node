package workload

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"github.com/pranavreddyg17/home-node/internal/state"
)

type regressingTransferBackend struct{ Backend }

func (b regressingTransferBackend) Call(ctx context.Context, instance string, request guestproto.Request) (guestproto.Response, error) {
	response, err := b.Backend.Call(ctx, instance, request)
	if err == nil && request.Operation == "upload" {
		response.Offset = request.Offset + int64(len(request.Data))
	}
	return response, err
}

type lostTransferReplyBackend struct {
	Backend
	operation string
}

func (b lostTransferReplyBackend) Call(ctx context.Context, instance string, request guestproto.Request) (guestproto.Response, error) {
	response, err := b.Backend.Call(ctx, instance, request)
	if err == nil && response.Error == "" && request.Operation == b.operation {
		return guestproto.Response{}, context.Canceled
	}
	return response, err
}

func TestTransferRecoversLostRepliesAfterControllerReopen(t *testing.T) {
	s, backend, device := service(t)
	ctx := context.Background()
	startFiles(t, s, device)
	var databasePath string
	if err := s.Store.DB.QueryRow("SELECT file FROM pragma_database_list WHERE name='main'").Scan(&databasePath); err != nil {
		t.Fatal(err)
	}
	reopen := func() {
		t.Helper()
		if err := s.Store.Close(); err != nil {
			t.Fatal(err)
		}
		store, err := state.Open(filepath.Dir(databasePath))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = store.Close() })
		s = New(store, backend, 1)
	}
	data := []byte("abcdefgh")
	transfer, err := s.CreateTransfer(ctx, device, "lost-reply.txt", int64(len(data)), sum(data))
	if err != nil {
		t.Fatal(err)
	}
	s.Backend = lostTransferReplyBackend{Backend: backend, operation: "upload"}
	if _, err := s.Upload(ctx, device, transfer.ID, 0, data[:4], sum(data[:4])); !errors.Is(err, context.Canceled) {
		t.Fatal("upload reply was not lost", err)
	}
	reopen()
	observed, err := s.Transfer(ctx, device, transfer.ID)
	if err != nil || observed.Offset != 0 {
		t.Fatal("unacknowledged offset was committed", observed, err)
	}
	wrong := []byte("xxxx")
	if _, err := s.Upload(ctx, device, transfer.ID, 0, wrong, sum(wrong)); !errors.Is(err, ErrConflict) {
		t.Fatal("conflicting replay accepted", err)
	}
	for offset := 0; offset < len(data); offset += 4 {
		chunk := data[offset : offset+4]
		if _, err := s.Upload(ctx, device, transfer.ID, int64(offset), chunk, sum(chunk)); err != nil {
			t.Fatal("matching replay failed", err)
		}
	}
	s.Backend = lostTransferReplyBackend{Backend: backend, operation: "finalize"}
	if _, err := s.Finalize(ctx, device, transfer.ID); !errors.Is(err, context.Canceled) {
		t.Fatal("finalization reply was not lost", err)
	}
	reopen()
	observed, err = s.Transfer(ctx, device, transfer.ID)
	if err != nil || observed.State != "verifying" || observed.Offset != int64(len(data)) {
		t.Fatal("finalization intent not retained", observed, err)
	}
	for range 2 {
		file, err := s.Finalize(ctx, device, transfer.ID)
		if err != nil || file.ID != transfer.ID || file.SHA256 != sum(data) {
			t.Fatal("finalization recovery failed", file, err)
		}
	}
	got, err := s.Download(ctx, transfer.ID, 0)
	if err != nil || string(got) != string(data) {
		t.Fatal("recovery changed uploaded bytes", err)
	}
	var count int
	if err := s.Store.DB.QueryRow("SELECT count(*) FROM files WHERE id=?", transfer.ID).Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate publication", count, err)
	}
}

func TestCancelledTransferRetainsQuotaAcrossLostDeletionReply(t *testing.T) {
	s, backend, device := service(t)
	ctx := context.Background()
	startFiles(t, s, device)
	data := []byte("discarded partial upload")
	transfer, err := s.CreateTransfer(ctx, device, "discard.txt", int64(len(data)), sum(data))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Upload(ctx, device, transfer.ID, 0, data, sum(data)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CancelTransfer(ctx, device, transfer.ID); err != nil {
		t.Fatal(err)
	}
	s.Backend = lostTransferReplyBackend{Backend: backend, operation: "delete"}
	if err := s.expireTransfer(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("deletion reply not lost", err)
	}
	var databasePath string
	if err := s.Store.DB.QueryRow("SELECT file FROM pragma_database_list WHERE name='main'").Scan(&databasePath); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := state.Open(filepath.Dir(databasePath))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	s = New(store, backend, 1)
	checkReservation := func(want int64) {
		t.Helper()
		var reserved int64
		if err := store.DB.QueryRow("SELECT coalesce(sum(size),0) FROM transfers WHERE state IN('uploading','verifying','cancelling')").Scan(&reserved); err != nil || reserved != want {
			t.Fatal("unexpected reservation", reserved, want, err)
		}
	}
	checkReservation(int64(len(data)))
	if _, err := s.Upload(ctx, device, transfer.ID, 0, data, sum(data)); !errors.Is(err, ErrConflict) {
		t.Fatal("cancelled upload resumed", err)
	}
	if _, err := s.Finalize(ctx, device, transfer.ID); !errors.Is(err, ErrConflict) {
		t.Fatal("cancelled upload finalized", err)
	}
	if err := s.expireTransfer(ctx); err != nil {
		t.Fatal("cleanup replay failed", err)
	}
	checkReservation(0)
	observed, err := s.Transfer(ctx, device, transfer.ID)
	if err != nil || observed.State != "cancelled" {
		t.Fatal("cancellation not acknowledged", observed, err)
	}
	if err := s.expireTransfer(ctx); err != nil {
		t.Fatal("completed cleanup replay failed", err)
	}
}

func TestUploadReplayCannotRewindAcknowledgedProgress(t *testing.T) {
	s, backend, device := service(t)
	ctx := context.Background()
	startFiles(t, s, device)
	data := []byte("abcdefgh")
	transfer, err := s.CreateTransfer(ctx, device, "resume.txt", int64(len(data)), sum(data))
	if err != nil {
		t.Fatal(err)
	}
	for offset := 0; offset < len(data); offset += 4 {
		chunk := data[offset : offset+4]
		if _, err := s.Upload(ctx, device, transfer.ID, int64(offset), chunk, sum(chunk)); err != nil {
			t.Fatal(err)
		}
	}
	s.Backend = regressingTransferBackend{Backend: backend}
	if _, err := s.Upload(ctx, device, transfer.ID, 0, data[:4], sum(data[:4])); !errors.Is(err, ErrConflict) {
		t.Fatal("regressing guest acknowledgement accepted", err)
	}
	retained, err := s.Transfer(ctx, device, transfer.ID)
	if err != nil || retained.Offset != int64(len(data)) || retained.State != "uploading" {
		t.Fatal("refused acknowledgement changed durable progress", retained, err)
	}
	s.Backend = backend
	if _, err := s.Upload(ctx, device, transfer.ID, 0, data[:4], sum(data[:4])); err != nil {
		t.Fatal("valid replay refused", err)
	}
	file, err := s.Finalize(ctx, device, transfer.ID)
	if err != nil || file.SHA256 != sum(data) {
		t.Fatal("replay damaged finalization", file, err)
	}
}
