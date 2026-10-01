package runtimeclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"unicode/utf8"
)

var ErrMaintenance = errors.New("local maintenance service unavailable or acknowledgement invalid")
var maintenanceID = regexp.MustCompile(`^[A-Za-z0-9_-]{20,64}$`)

// MaintenanceClient has no general runtime or guest operation methods. Its
// socket must be installed with the distinct backup peer's access permissions.
type MaintenanceClient struct{ client *http.Client }

func NewMaintenance(socket string) *MaintenanceClient {
	return &MaintenanceClient{client: socketClient(socket)}
}
func (c *MaintenanceClient) BeginRuntimeMaintenanceForJob(ctx context.Context, job string) (string, error) {
	if !maintenanceID.MatchString(job) {
		return "", ErrMaintenance
	}
	return c.call(ctx, "/v1/maintenance/begin", map[string]any{"version": 1, "jobId": job}, true)
}
func (c *MaintenanceClient) EndRuntimeMaintenance(ctx context.Context, token string) error {
	if !maintenanceID.MatchString(token) {
		return ErrMaintenance
	}
	_, err := c.call(ctx, "/v1/maintenance/end", map[string]any{"version": 1, "token": token}, false)
	return err
}
func (c *MaintenanceClient) call(ctx context.Context, path string, payload any, acquire bool) (string, error) {
	if c == nil || c.client == nil {
		return "", ErrMaintenance
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", ErrMaintenance
	}
	request, err := http.NewRequestWithContext(ctx, "POST", "http://local"+path, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.client.Do(request)
	if err != nil {
		return "", errors.Join(ErrMaintenance, ctx.Err())
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return "", ErrMaintenance
	}
	data, err = io.ReadAll(io.LimitReader(response.Body, 513))
	if err != nil || len(data) > 512 || !utf8.Valid(data) {
		return "", ErrMaintenance
	}
	return decodeMaintenanceReply(data, acquire)
}
func decodeMaintenanceReply(data []byte, acquire bool) (string, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return "", ErrMaintenance
	}
	fields := map[string]json.RawMessage{}
	for decoder.More() {
		key, err := decoder.Token()
		name, ok := key.(string)
		if err != nil || !ok || fields[name] != nil || (name != "version" && name != "token") {
			return "", ErrMaintenance
		}
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return "", ErrMaintenance
		}
		fields[name] = value
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') || decoder.Decode(new(any)) != io.EOF {
		return "", ErrMaintenance
	}
	var version int
	if json.Unmarshal(fields["version"], &version) != nil || version != 1 {
		return "", ErrMaintenance
	}
	if !acquire {
		if len(fields) != 1 {
			return "", ErrMaintenance
		}
		return "", nil
	}
	var token string
	if len(fields) != 2 || json.Unmarshal(fields["token"], &token) != nil || !maintenanceID.MatchString(token) {
		return "", ErrMaintenance
	}
	return token, nil
}
