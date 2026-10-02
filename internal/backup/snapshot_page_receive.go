package backup

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/pranavreddyg17/home-node/internal/disktransport"
	"github.com/pranavreddyg17/home-node/internal/guestproto"
)

// Caller must authenticate the configured peer before receiving candidate data.
// This receiver establishes request correlation, not peer or restore authority.
func receiveSnapshotPageResponse(ctx context.Context, connection *net.UnixConn, requestID string) (SnapshotPage, error) {
	if err := ctx.Err(); err != nil {
		return SnapshotPage{}, err
	}
	if !guestproto.ValidID(requestID) {
		return SnapshotPage{}, ErrManifest
	}
	deadline, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	raw, err := disktransport.ReceivePacket(deadline, connection)
	if err != nil {
		return SnapshotPage{}, errors.Join(ErrManifest, deadline.Err())
	}
	if err := deadline.Err(); err != nil {
		return SnapshotPage{}, err
	}
	page, err := DecodeSnapshotPageResponse(raw, requestID)
	if err != nil {
		return SnapshotPage{}, err
	}
	if err := deadline.Err(); err != nil {
		return SnapshotPage{}, err
	}
	return page, nil
}
