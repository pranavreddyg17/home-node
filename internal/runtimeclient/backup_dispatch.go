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

// SnapshotPage uses only the installed credential socket. Device authorization
// must be established separately; this transport cannot establish owner access.
func (d ActivatedBackupDispatcher) SnapshotPage(ctx context.Context, request backup.SnapshotPageRequest, credential *os.File) (backup.SnapshotPage, error) {
	if err := ctx.Err(); err != nil {
		return backup.SnapshotPage{}, err
	}
	if _, err := backup.EncodeSnapshotPageRequest(request); err != nil {
		return backup.SnapshotPage{}, err
	}
	var page backup.SnapshotPage
	err := d.deliver(ctx, credential, func(connection *net.UnixConn) error {
		var err error
		page, err = backup.SendActivatedCredentialSnapshotPageAndWait(ctx, connection, request, credential)
		return err
	})
	if err != nil {
		return backup.SnapshotPage{}, err
	}
	return page, nil
}

// SnapshotPreview inspects only the selected snapshot metadata. Exact pending
// request authorization must be supplied independently by the controller.
func (d ActivatedBackupDispatcher) SnapshotPreview(ctx context.Context, request backup.SnapshotPreviewRequest, credential *os.File) (backup.SnapshotPreview, error) {
	if err := ctx.Err(); err != nil {
		return backup.SnapshotPreview{}, err
	}
	if _, err := backup.EncodeSnapshotPreviewRequest(request); err != nil {
		return backup.SnapshotPreview{}, err
	}
	var preview backup.SnapshotPreview
	err := d.deliver(ctx, credential, func(connection *net.UnixConn) error {
		var err error
		preview, err = backup.SendActivatedCredentialSnapshotPreviewAndWait(ctx, connection, request, credential)
		return err
	})
	if err != nil {
		return backup.SnapshotPreview{}, err
	}
	return preview, nil
}
