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

// SendDispatch sends one job packet to an authenticated non-root backup peer.
// Success means only that the packet was sent, never that the worker admitted,
// published, or cleaned up the job. Durable controller outcome records provide
// publication evidence. Caller retains exclusive connection ownership/closure.
func SendDispatch(ctx context.Context, connection *net.UnixConn, backupUID uint32, dispatch Dispatch) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	raw, err := EncodeDispatch(dispatch)
	if err != nil {
		return err
	}
	if connection == nil || backupUID == 0 || connection.LocalAddr().Network() != "unixpacket" {
		return ErrManifest
	}
	peer, err := supervisor.PeerUID(connection)
	if err != nil || peer != backupUID {
		return ErrManifest
	}
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err = disktransport.SendPacket(bounded, connection, raw); err != nil {
		return errors.Join(ErrManifest, bounded.Err())
	}
	return bounded.Err()
}
