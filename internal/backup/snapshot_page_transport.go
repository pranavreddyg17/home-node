package backup

import (
	"context"
	"net"
	"os"
)

// SendActivatedCredentialSnapshotPage uses only the configured, protected,
// root-created systemd listener. It authenticates its creator before handing
// over a sealed read-only credential. The caller retains its descriptor.
func SendActivatedCredentialSnapshotPage(ctx context.Context, connection *net.UnixConn, request SnapshotPageRequest, credential *os.File) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	raw, err := EncodeSnapshotPageRequest(request)
	if err != nil {
		return err
	}
	return sendCredentialPayload(ctx, connection, 0, raw, credential)
}

// ReceiveCredentialSnapshotPage authenticates the controller before receipt.
// Success transfers ownership of the received credential to the caller, which
// must independently authorize the claimed device before repository access.
func ReceiveCredentialSnapshotPage(ctx context.Context, connection *net.UnixConn, controllerUID uint32) (SnapshotPageRequest, *os.File, error) {
	return receiveCredentialPayload(ctx, connection, controllerUID, DecodeSnapshotPageRequest)
}

// SendActivatedCredentialSnapshotPageAndWait authenticates the installed
// listener before sending and accepts only the response to this request.
// A returned page contains candidates, not authority to restore them.
func SendActivatedCredentialSnapshotPageAndWait(ctx context.Context, connection *net.UnixConn, request SnapshotPageRequest, credential *os.File) (SnapshotPage, error) {
	if err := SendActivatedCredentialSnapshotPage(ctx, connection, request, credential); err != nil {
		return SnapshotPage{}, err
	}
	return receiveSnapshotPageResponse(ctx, connection, request.RequestID)
}
