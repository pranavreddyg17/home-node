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
	if !filepath.IsAbs(d.Socket) || filepath.Clean(d.Socket) != d.Socket || d.ControllerGID < 100 || d.ControllerGID > 999 || credential == nil {
		return backup.ErrManifest
	}
	if _, err := backup.EncodeDispatch(job); err != nil {
		return err
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
	return backup.SendActivatedCredentialDispatchAndWait(ctx, packet, job, credential)
}
