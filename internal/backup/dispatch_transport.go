package backup

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/pranavreddyg17/home-node/internal/disktransport"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

// ReceiveDispatch consumes one bounded packet from an exclusively owned
// accepted unixpacket connection. It authenticates the configured non-root
// controller before reading job data. It accepts no file descriptors, and the
// shared transport closes unexpected descriptors atomically marked close-on-exec.
// The caller owns connection closure and worker admission/concurrency limits.
func ReceiveDispatch(ctx context.Context, connection *net.UnixConn, controllerUID uint32) (Dispatch, error) {
	if err := ctx.Err(); err != nil {
		return Dispatch{}, err
	}
	if connection == nil || controllerUID == 0 || connection.LocalAddr().Network() != "unixpacket" {
		return Dispatch{}, ErrManifest
	}
	peer, err := supervisor.PeerUID(connection)
	if err != nil || peer != controllerUID {
		return Dispatch{}, ErrManifest
	}
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	raw, err := disktransport.ReceivePacket(bounded, connection)
	if err != nil {
		return Dispatch{}, errors.Join(ErrManifest, bounded.Err())
	}
	if err = bounded.Err(); err != nil {
		return Dispatch{}, err
	}
	return DecodeDispatch(raw)
}
