//go:build linux

package backup

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestCredentialServiceRoutesPreviewAndClosesBeforeReply(t *testing.T) {
	uid := uint32(os.Geteuid())
	if uid == 0 {
		t.Skip("controller must be unprivileged")
	}
	listener, err := net.Listen("unixpacket", filepath.Join(t.TempDir(), "service.sock"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	request := SnapshotPreviewRequest{Version: 5, Kind: "snapshot-preview", RequestID: state.Random(), DeviceID: state.Random(), SnapshotID: state.Hash("selected")}
	received := make(chan *os.File, 1)
	done := make(chan error, 1)
	go func() {
		done <- ServeCredentialService(ctx, listener, uid, func(context.Context, WorkerRequest, *os.File) error {
			return errors.New("metadata routed to maintenance")
		}, func(context.Context, SnapshotPageRequest, *os.File) (SnapshotPage, error) {
			return SnapshotPage{}, errors.New("preview routed to listing")
		}, func(ctx context.Context, got SnapshotPreviewRequest, file *os.File) (SnapshotPreview, error) {
			if got != request {
				return SnapshotPreview{}, ErrManifest
			}
			secret, err := ReadRepositoryPassword(ctx, file)
			if err != nil {
				return SnapshotPreview{}, err
			}
			defer clear(secret)
			if string(secret) != "fixture-service-secret" {
				return SnapshotPreview{}, ErrManifest
			}
			received <- file
			return SnapshotPreview{SnapshotID: request.SnapshotID, CreatedAt: time.Now(), Release: "0.1.0", CatalogVersion: 1, Validation: "metadata-compatible", Files: []SnapshotPreviewFile{{Workload: "management", Bytes: 1024}}}, nil
		})
	}()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	connection, err := (&net.Dialer{}).DialContext(ctx, "unixpacket", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	credential, err := CreateRepositoryPassword([]byte("fixture-service-secret"))
	if err != nil {
		t.Fatal(err)
	}
	defer credential.Close()
	raw, err := EncodeSnapshotPreviewRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	packet := connection.(*net.UnixConn)
	if err := sendCredentialPayload(ctx, packet, uid, raw, credential); err != nil {
		t.Fatal(err)
	}
	page, err := receiveSnapshotPreviewResponse(ctx, packet, request.RequestID, request.SnapshotID)
	if err != nil || page.Files == nil || page.SnapshotID != request.SnapshotID {
		t.Fatal("metadata response lost", page, err)
	}
	select {
	case file := <-received:
		if _, err := file.Stat(); err == nil {
			t.Fatal("response preceded credential closure")
		}
	default:
		t.Fatal("metadata callback did not execute")
	}
	if _, err := credential.Stat(); err != nil {
		t.Fatal("worker closed caller descriptor", err)
	}
}
