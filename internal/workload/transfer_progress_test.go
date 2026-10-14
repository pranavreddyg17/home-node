package workload

import (
	"context"
	"errors"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
)

type regressingTransferBackend struct{ Backend }

func (b regressingTransferBackend) Call(ctx context.Context, instance string, request guestproto.Request) (guestproto.Response, error) {
	response, err := b.Backend.Call(ctx, instance, request)
	if err == nil && request.Operation == "upload" {
		response.Offset = request.Offset + int64(len(request.Data))
	}
	return response, err
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
