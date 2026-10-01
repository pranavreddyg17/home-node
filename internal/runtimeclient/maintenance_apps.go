package runtimeclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"regexp"
	"time"
	"unicode/utf8"

	"github.com/pranavreddyg17/home-node/internal/state"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

// MaintenanceAppsClient uses only fixed private maintenance endpoints. Inspect must resolve
// the existing coordinator-owned job; it cannot create a new admission barrier.
type MaintenanceAppsClient struct {
	client  *http.Client
	inspect func(context.Context, string) (state.MaintenanceJob, error)
}

func NewMaintenanceApps(socket string, controllerUID uint32, inspect func(context.Context, string) (state.MaintenanceJob, error)) *MaintenanceAppsClient {
	if controllerUID == 0 {
		return &MaintenanceAppsClient{inspect: inspect}
	}
	return newMaintenanceApps(socket, controllerUID, inspect)
}

// NewActivatedMaintenanceApps authenticates the root creator of a systemd
// listener inherited by the controller. SO_PEERCRED reports that creator, not
// the current accepting process. Use only the installed backup-only socket.
func NewActivatedMaintenanceApps(socket string, inspect func(context.Context, string) (state.MaintenanceJob, error)) *MaintenanceAppsClient {
	return newMaintenanceApps(socket, 0, inspect)
}
func newMaintenanceApps(socket string, listenerUID uint32, inspect func(context.Context, string) (state.MaintenanceJob, error)) *MaintenanceAppsClient {
	c := &MaintenanceAppsClient{inspect: inspect}
	if inspect == nil || !filepath.IsAbs(socket) || filepath.Clean(socket) != socket {
		return c
	}
	c.client = &http.Client{Timeout: 185 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }, Transport: &http.Transport{MaxConnsPerHost: 2, MaxIdleConns: 1, IdleConnTimeout: 30 * time.Second, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		connection, err := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "unix", socket)
		if err != nil {
			return nil, err
		}
		unix, ok := connection.(*net.UnixConn)
		if !ok {
			connection.Close()
			return nil, ErrMaintenance
		}
		peer, err := supervisor.PeerUID(unix)
		if err != nil || peer != listenerUID {
			connection.Close()
			return nil, ErrMaintenance
		}
		return connection, nil
	}}}
	return c
}
func (c *MaintenanceAppsClient) DrainMaintenance(ctx context.Context, token, device string) error {
	return c.call(ctx, "/v1/maintenance/drain", token, device)
}
func (c *MaintenanceAppsClient) RestoreMaintenanceApps(ctx context.Context, token, device string) error {
	return c.call(ctx, "/v1/maintenance/restore", token, device)
}
func (c *MaintenanceAppsClient) call(ctx context.Context, path, token, device string) error {
	return c.callPayload(ctx, path, token, device, "")
}
func (c *MaintenanceAppsClient) callPayload(ctx context.Context, path, token, device, snapshot string) error {
	if c == nil || c.client == nil || c.inspect == nil || !maintenanceID.MatchString(token) || !maintenanceID.MatchString(device) {
		return ErrMaintenance
	}
	job, err := c.inspect(ctx, token)
	if err != nil || job.Device != device || !maintenanceID.MatchString(job.ID) {
		return ErrMaintenance
	}
	payload := map[string]any{"version": 1, "token": token, "jobId": job.ID, "deviceId": device}
	if snapshot != "" {
		payload["snapshotId"] = snapshot
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return ErrMaintenance
	}
	request, err := http.NewRequestWithContext(ctx, "POST", "http://local"+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.client.Do(request)
	if err != nil {
		return errors.Join(ErrMaintenance, ctx.Err())
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return ErrMaintenance
	}
	data, err = io.ReadAll(io.LimitReader(response.Body, 513))
	if err != nil || len(data) > 512 || !utf8.Valid(data) {
		return ErrMaintenance
	}
	_, err = decodeMaintenanceReply(data, false)
	return err
}

func (c *MaintenanceAppsClient) Close() {
	if c != nil && c.client != nil {
		c.client.CloseIdleConnections()
	}
}

// ConfirmStaging verifies the existing owned staging job and drained inventory;
// it neither acquires nor releases any maintenance authority.
func (c *MaintenanceAppsClient) ConfirmStaging(ctx context.Context, token, device string) error {
	return c.call(ctx, "/v1/maintenance/verify-stage", token, device)
}

func (c *MaintenanceAppsClient) BeginPublishing(ctx context.Context, token, device string) error {
	return c.call(ctx, "/v1/maintenance/begin-publish", token, device)
}
func (c *MaintenanceAppsClient) ConfirmPublishing(ctx context.Context, token, device string) error {
	return c.call(ctx, "/v1/maintenance/verify-publish", token, device)
}

func (c *MaintenanceAppsClient) ClaimPublication(ctx context.Context, token, device string) error {
	return c.call(ctx, "/v1/maintenance/claim-publish", token, device)
}

var publicationSnapshotID = regexp.MustCompile(`^[a-f0-9]{64}$`)

func (c *MaintenanceAppsClient) RecordPublication(ctx context.Context, token, device, snapshot string) error {
	if !publicationSnapshotID.MatchString(snapshot) {
		return ErrMaintenance
	}
	return c.callPayload(ctx, "/v1/maintenance/ack-publish", token, device, snapshot)
}
