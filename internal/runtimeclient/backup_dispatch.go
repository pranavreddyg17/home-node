package runtimeclient

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/pranavreddyg17/home-node/internal/backup"
)

// ActivatedBackupDispatcher is configured by the installed controller, never
// by an HTTP request. It admits only the protected controller-group credential
// socket and delegates root-creator authentication and completion validation.
// It does not retry after any error, because the worker may have begun effects.
type ActivatedBackupDispatcher struct {
	Socket        string
	ControllerGID uint32
}

func (d ActivatedBackupDispatcher) Deliver(ctx context.Context, job backup.Dispatch, credential *os.File) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := backup.EncodeDispatch(job); err != nil {
		return err
	}
	return d.deliver(ctx, credential, func(connection *net.UnixConn) error {
		return backup.SendActivatedCredentialDispatchAndWait(ctx, connection, job, credential)
	})
}
func (d ActivatedBackupDispatcher) deliver(ctx context.Context, credential *os.File, send func(*net.UnixConn) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !filepath.IsAbs(d.Socket) || filepath.Clean(d.Socket) != d.Socket || d.ControllerGID < 100 || d.ControllerGID > 999 || credential == nil || send == nil {
		return backup.ErrManifest
	}
	if err := validateBackupCredentialSocket(d.Socket, d.ControllerGID); err != nil {
		return err
	}
	connection, err := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "unixpacket", d.Socket)
	if err != nil {
		return err
	}
	defer connection.Close()
	packet, ok := connection.(*net.UnixConn)
	if !ok {
		return backup.ErrManifest
	}
	return send(packet)
}

func (d ActivatedBackupDispatcher) DeliverLaunch(ctx context.Context, launch backup.Launch, credential *os.File) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := backup.EncodeLaunch(launch); err != nil {
		return err
	}
	return d.deliver(ctx, credential, func(connection *net.UnixConn) error {
		return backup.SendActivatedCredentialLaunchAndWait(ctx, connection, launch, credential)
	})
}
func (d ActivatedBackupDispatcher) DeliverCleanup(ctx context.Context, cleanup backup.Cleanup, credential *os.File) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := backup.EncodeCleanup(cleanup); err != nil {
		return err
	}
	return d.deliver(ctx, credential, func(connection *net.UnixConn) error {
		return backup.SendActivatedCredentialCleanupAndWait(ctx, connection, cleanup, credential)
	})
}
