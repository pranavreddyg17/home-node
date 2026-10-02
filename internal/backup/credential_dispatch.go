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
	if backupUID == 0 {
		return ErrManifest
	}
	return sendCredentialDispatch(ctx, connection, backupUID, dispatch, credential)
}

// SendActivatedCredentialDispatch authenticates the root creator of the known
// installed systemd credential listener inherited by the unprivileged worker.
// SO_PEERCRED identifies that creator rather than the accepting process. Use
// only with the configured protected listener, never as fallback after refusal.
func SendActivatedCredentialDispatch(ctx context.Context, connection *net.UnixConn, dispatch Dispatch, credential *os.File) error {
	return sendCredentialDispatch(ctx, connection, 0, dispatch, credential)
}
func sendCredentialDispatch(ctx context.Context, connection *net.UnixConn, expectedUID uint32, dispatch Dispatch, credential *os.File) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	raw, err := EncodeDispatch(dispatch)
	if err != nil {
		return err
	}
	return sendCredentialPayload(ctx, connection, expectedUID, raw, credential)
}
func sendCredentialPayload(ctx context.Context, connection *net.UnixConn, expectedUID uint32, raw []byte, credential *os.File) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if connection == nil || connection.LocalAddr().Network() != "unixpacket" {
		return ErrManifest
	}
	peer, err := supervisor.PeerUID(connection)
	if err != nil || peer != expectedUID {
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
func ReceiveCredentialDispatch(ctx context.Context, connection *net.UnixConn, controllerUID uint32) (Dispatch, *os.File, error) {
	return receiveCredentialPayload(ctx, connection, controllerUID, DecodeDispatch)
}
func receiveCredentialPayload[T any](ctx context.Context, connection *net.UnixConn, controllerUID uint32, decode func([]byte) (T, error)) (message T, credential *os.File, resultErr error) {
	var zero T

	if err := ctx.Err(); err != nil {
		return zero, nil, err
	}
	if connection == nil || controllerUID == 0 || connection.LocalAddr().Network() != "unixpacket" {
		return zero, nil, ErrManifest
	}
	peer, err := supervisor.PeerUID(connection)
	if err != nil || peer != controllerUID {
		return zero, nil, ErrManifest
	}
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	raw, file, err := disktransport.ReceiveFile(bounded, connection)
	if err != nil {
		return zero, nil, errors.Join(ErrManifest, bounded.Err())
	}
	accepted := false
	defer func() {
		if !accepted {
			file.Close()
		}
	}()
	message, err = decode(raw)
	if err != nil {
		return zero, nil, err
	}
	password, err := ReadRepositoryPassword(bounded, file)
	if err != nil {
		return zero, nil, err
	}
	clear(password)
	if err = bounded.Err(); err != nil {
		return zero, nil, err
	}
	accepted = true
	return message, file, nil
}

// SendActivatedCredentialLaunch sends preliminary job data to the installed
// root-created worker listener; it carries one sealed credential and no root token.
func SendActivatedCredentialLaunch(ctx context.Context, connection *net.UnixConn, launch Launch, credential *os.File) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	raw, err := EncodeLaunch(launch)
	if err != nil {
		return err
	}
	return sendCredentialPayload(ctx, connection, 0, raw, credential)
}
func ReceiveCredentialLaunch(ctx context.Context, connection *net.UnixConn, controllerUID uint32) (Launch, *os.File, error) {
	return receiveCredentialPayload(ctx, connection, controllerUID, DecodeLaunch)
}
