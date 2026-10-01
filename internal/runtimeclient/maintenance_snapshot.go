package runtimeclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"

	"github.com/pranavreddyg17/home-node/internal/state"
)

// StageManagementSnapshot receives only the sanitized owned-job recovery export.
// The caller must exclusively own the private staging root through this call.
// No live management database path or maintenance acquisition is exposed.
func (c *MaintenanceAppsClient) StageManagementSnapshot(ctx context.Context, token, device string, root *os.Root) (resultErr error) {
	if c == nil || c.client == nil || c.inspect == nil || root == nil || !maintenanceID.MatchString(token) || !maintenanceID.MatchString(device) {
		return ErrMaintenance
	}
	job, err := c.inspect(ctx, token)
	if err != nil || job.Device != device || !maintenanceID.MatchString(job.ID) {
		return ErrMaintenance
	}
	info, err := root.Stat(".")
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return ErrMaintenance
	}
	data, err := json.Marshal(map[string]any{"version": 1, "token": token, "jobId": job.ID, "deviceId": device})
	if err != nil {
		return ErrMaintenance
	}
	request, err := http.NewRequestWithContext(ctx, "POST", "http://local/v1/maintenance/snapshot", bytes.NewReader(data))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.client.Do(request)
	if err != nil {
		return errors.Join(ErrMaintenance, ctx.Err())
	}
	defer response.Body.Close()
	if response.StatusCode != 200 || response.Header.Get("Content-Type") != "application/vnd.homenode.recovery-snapshot" || response.ContentLength <= 0 || response.ContentLength > 256<<20 {
		return ErrMaintenance
	}
	directory, err := root.Open(".")
	if err != nil {
		return err
	}
	defer directory.Close()
	file, err := root.OpenFile("snapshot.db", os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	original, err := file.Stat()
	if err != nil {
		file.Close()
		return err
	}
	defer func() {
		resultErr = errors.Join(resultErr, file.Close())
		if resultErr != nil {
			current, err := root.Lstat("snapshot.db")
			if err == nil && os.SameFile(original, current) {
				resultErr = errors.Join(resultErr, root.Remove("snapshot.db"), directory.Sync())
			} else if err == nil {
				resultErr = errors.Join(resultErr, ErrMaintenance)
			}
		}
	}()
	remaining := response.ContentLength
	buffer := make([]byte, 64<<10)
	for remaining > 0 {
		if err = ctx.Err(); err != nil {
			return err
		}
		n, err := io.ReadFull(response.Body, buffer[:min(remaining, int64(len(buffer)))])
		if err != nil {
			return errors.Join(ErrMaintenance, err)
		}
		written, err := file.Write(buffer[:n])
		if err != nil {
			return err
		}
		if written != n {
			return io.ErrShortWrite
		}
		remaining -= int64(n)
	}
	var extra [1]byte
	if n, err := response.Body.Read(extra[:]); n != 0 || err != io.EOF {
		return ErrMaintenance
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if _, err = state.ValidateRecoverySnapshot(ctx, file); err != nil {
		return errors.Join(ErrMaintenance, err)
	}
	if err = directory.Sync(); err != nil {
		return err
	}
	return ctx.Err()
}
