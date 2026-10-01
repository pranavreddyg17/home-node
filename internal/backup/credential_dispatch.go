package backup

import (
	"context"
	"errors"
	"net"
	"os"
	"time"

	"github.com/pranavreddyg17/home-node/internal/disktransport"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

// SendCredentialDispatch belongs on a separate authenticated credential handoff
// channel. It sends one job plus one immutable read-only descriptor. Caller
// retains ownership of its credential and connection; delivery is not backup
// completion. Ordinary SendDispatch/ReceiveDispatch still prohibit descriptors.
func SendCredentialDispatch(ctx context.Context, connection *net.UnixConn, backupUID uint32, dispatch Dispatch, credential *os.File) error {
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
	password, err := ReadRepositoryPassword(ctx, credential)
	if err != nil {
		return err
	}
	clear(password)
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err = disktransport.SendFile(bounded, connection, raw, credential); err != nil {
		return errors.Join(ErrManifest, bounded.Err())
	}
	return bounded.Err()
}

// ReceiveCredentialDispatch authenticates the non-root controller before
// receiving exactly one close-on-exec descriptor. All rejection paths close
// the received duplicate; success transfers closure responsibility to caller.
func ReceiveCredentialDispatch(ctx context.Context, connection *net.UnixConn, controllerUID uint32) (dispatch Dispatch, credential *os.File, resultErr error) {
	if err := ctx.Err(); err != nil {
		return Dispatch{}, nil, err
	}
	if connection == nil || controllerUID == 0 || connection.LocalAddr().Network() != "unixpacket" {
		return Dispatch{}, nil, ErrManifest
	}
	peer, err := supervisor.PeerUID(connection)
	if err != nil || peer != controllerUID {
		return Dispatch{}, nil, ErrManifest
	}
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	raw, file, err := disktransport.ReceiveFile(bounded, connection)
	if err != nil {
		return Dispatch{}, nil, errors.Join(ErrManifest, bounded.Err())
	}
	accepted := false
	defer func() {
		if !accepted {
			file.Close()
		}
	}()
	dispatch, err = DecodeDispatch(raw)
	if err != nil {
		return Dispatch{}, nil, err
	}
	password, err := ReadRepositoryPassword(bounded, file)
	if err != nil {
		return Dispatch{}, nil, err
	}
	clear(password)
	if err = bounded.Err(); err != nil {
		return Dispatch{}, nil, err
	}
	accepted = true
	return dispatch, file, nil
}
