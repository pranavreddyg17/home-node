package backup

import (
	"bytes"
	"encoding/json"
	"io"
	"unicode/utf8"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
)

const MaxDispatchBytes = 1024

// Dispatch is controller-to-backup job data, not authority by itself. Transport
// must authenticate the controller kernel identity; the private controller and
// root disk services independently authenticate each subsequent operation.
// Paths, executables, repository credentials, and restore policy are configured
// by trusted service setup and are deliberately absent from this message.
type Dispatch struct {
	Version         int    `json:"version"`
	JobID           string `json:"jobId"`
	DeviceID        string `json:"deviceId"`
	ManagementToken string `json:"managementToken"`
	RuntimeToken    string `json:"runtimeToken"`
	Release         string `json:"release"`
	CatalogVersion  int64  `json:"catalogVersion"`
}

func (d Dispatch) valid() bool {
	return d.Version == 1 && guestproto.ValidID(d.JobID) && guestproto.ValidID(d.DeviceID) && guestproto.ValidID(d.ManagementToken) && guestproto.ValidID(d.RuntimeToken) && releasePattern.MatchString(d.Release) && d.CatalogVersion >= 1
}
func EncodeDispatch(d Dispatch) ([]byte, error) {
	if !d.valid() {
		return nil, ErrManifest
	}
	raw, err := json.Marshal(d)
	if err != nil || len(raw) > MaxDispatchBytes {
		return nil, ErrManifest
	}
	return raw, nil
}

// DecodeDispatch rejects duplicate decoded keys (including escaped aliases),
// unknown/missing fields, nulls, invalid IDs, and trailing JSON. It never logs
// tokens or includes the untrusted message in an error.
func DecodeDispatch(raw []byte) (Dispatch, error) {
	var d Dispatch
	if len(raw) > MaxDispatchBytes || !utf8.Valid(raw) {
		return d, ErrManifest
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return d, ErrManifest
	}
	seen := map[string]bool{}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return Dispatch{}, ErrManifest
		}
		name, ok := key.(string)
		if !ok || seen[name] {
			return Dispatch{}, ErrManifest
		}
		switch name {
		case "version", "jobId", "deviceId", "managementToken", "runtimeToken", "release", "catalogVersion":
		default:
			return Dispatch{}, ErrManifest
		}
		seen[name] = true
		var value json.RawMessage
		if decoder.Decode(&value) != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return Dispatch{}, ErrManifest
		}
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim('}') || len(seen) != 7 || decoder.Decode(new(any)) != io.EOF || json.Unmarshal(raw, &d) != nil || !d.valid() {
		return Dispatch{}, ErrManifest
	}
	return d, nil
}
