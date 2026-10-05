package backup

import (
	"context"
	"errors"
	"net"
	"os"
	"time"

	"github.com/pranavreddyg17/home-node/internal/disktransport"
	"github.com/pranavreddyg17/home-node/internal/guestproto"
)

// SendActivatedCredentialSnapshotPreview uses only the configured protected
// root-created systemd listener. Caller retains the sealed credential handle.
func SendActivatedCredentialSnapshotPreview(ctx context.Context, connection *net.UnixConn, request SnapshotPreviewRequest, credential *os.File) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	raw, err := EncodeSnapshotPreviewRequest(request)
	if err != nil {
		return err
	}
	return sendCredentialPayload(ctx, connection, 0, raw, credential)
}

// ReceiveCredentialSnapshotPreview authenticates the controller peer but not
// the claimed device. Caller owns closing the credential on success and must
// independently authorize the exact pending preview before repository access.
func ReceiveCredentialSnapshotPreview(ctx context.Context, connection *net.UnixConn, controllerUID uint32) (SnapshotPreviewRequest, *os.File, error) {
	return receiveCredentialPayload(ctx, connection, controllerUID, DecodeSnapshotPreviewRequest)
}

func SendActivatedCredentialSnapshotPreviewAndWait(ctx context.Context, connection *net.UnixConn, request SnapshotPreviewRequest, credential *os.File) (SnapshotPreview, error) {
	if err := SendActivatedCredentialSnapshotPreview(ctx, connection, request, credential); err != nil {
		return SnapshotPreview{}, err
	}
	return receiveSnapshotPreviewResponse(ctx, connection, request.RequestID, request.SnapshotID)
}

// Caller authenticates the configured peer before this correlation check.
func receiveSnapshotPreviewResponse(ctx context.Context, connection *net.UnixConn, requestID, snapshotID string) (SnapshotPreview, error) {
	if err := ctx.Err(); err != nil {
		return SnapshotPreview{}, err
	}
	if !guestproto.ValidID(requestID) || !repositoryPattern.MatchString(snapshotID) {
		return SnapshotPreview{}, ErrManifest
	}
	bounded, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	raw, err := disktransport.ReceivePacket(bounded, connection)
	if err != nil {
		return SnapshotPreview{}, errors.Join(ErrManifest, bounded.Err())
	}
	if err := bounded.Err(); err != nil {
		return SnapshotPreview{}, err
	}
	preview, err := DecodeSnapshotPreviewResponse(raw, requestID, snapshotID)
	if err != nil {
		return SnapshotPreview{}, err
	}
	if err := bounded.Err(); err != nil {
		return SnapshotPreview{}, err
	}
	return preview, nil
}
