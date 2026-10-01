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
	"time"
	"unicode/utf8"

	"github.com/pranavreddyg17/home-node/internal/state"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

// MaintenanceAppsClient has only drain/restore authority. Inspect must resolve
// the existing coordinator-owned job; it cannot create a new admission barrier.
type MaintenanceAppsClient struct {
	client  *http.Client
	inspect func(context.Context, string) (state.MaintenanceJob, error)
}

func NewMaintenanceApps(socket string, controllerUID uint32, inspect func(context.Context, string) (state.MaintenanceJob, error)) *MaintenanceAppsClient {
	c := &MaintenanceAppsClient{inspect: inspect}
	if controllerUID == 0 || inspect == nil || !filepath.IsAbs(socket) || filepath.Clean(socket) != socket {
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
		if err != nil || peer != controllerUID {
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
	if c == nil || c.client == nil || c.inspect == nil || !maintenanceID.MatchString(token) || !maintenanceID.MatchString(device) {
		return ErrMaintenance
	}
	job, err := c.inspect(ctx, token)
	if err != nil || job.Device != device || !maintenanceID.MatchString(job.ID) {
		return ErrMaintenance
	}
	data, err := json.Marshal(map[string]any{"version": 1, "token": token, "jobId": job.ID, "deviceId": device})
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
