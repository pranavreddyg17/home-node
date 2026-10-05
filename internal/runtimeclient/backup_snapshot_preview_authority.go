package runtimeclient

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/pranavreddyg17/home-node/internal/backup"
	"github.com/pranavreddyg17/home-node/internal/state"
)

// SnapshotPreviewAuthorityClient exposes no maintenance or runtime commands.
// Socket and listener UID come only from trusted installed configuration;
// zero UID denotes the root-created systemd management listener.
type SnapshotPreviewAuthorityClient struct{ transport *MaintenanceAppsClient }

func NewSnapshotPreviewAuthority(socket string, listenerUID uint32) *SnapshotPreviewAuthorityClient {
	return &SnapshotPreviewAuthorityClient{transport: newMaintenanceApps(socket, listenerUID, func(context.Context, string) (state.MaintenanceJob, error) {
		return state.MaintenanceJob{}, ErrMaintenance
	})}
}

func (c *SnapshotPreviewAuthorityClient) Close() {
	if c != nil && c.transport != nil {
		c.transport.Close()
	}
}

func (c *SnapshotPreviewAuthorityClient) VerifySnapshotPreview(ctx context.Context, request backup.SnapshotPreviewRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c == nil || c.transport == nil || c.transport.client == nil {
		return ErrMaintenance
	}
	raw, err := backup.EncodeSnapshotPreviewRequest(request)
	if err != nil {
		return err
	}
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(bounded, "POST", "http://local/v1/maintenance/verify-snapshot-preview", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := c.transport.client.Do(req)
	if err != nil {
		return errors.Join(ErrMaintenance, bounded.Err())
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return ErrMaintenance
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 513))
	if err != nil || len(data) > 512 {
		return errors.Join(ErrMaintenance, bounded.Err())
	}
	if _, err := decodeMaintenanceReply(data, false); err != nil {
		return err
	}
	return bounded.Err()
}
