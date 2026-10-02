package backup

import (
	"bytes"
	"encoding/json"
	"io"
	"unicode/utf8"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
)

// Launch names an already approved management job before runtime acquisition.
// Version2 is deliberately disjoint from Dispatch. It supplies no root token,
// destination, executable or password; authenticated worker transport is required.
type Launch struct {
	Version         int    `json:"version"`
	JobID           string `json:"jobId"`
	DeviceID        string `json:"deviceId"`
	ManagementToken string `json:"managementToken"`
	Release         string `json:"release"`
	CatalogVersion  int64  `json:"catalogVersion"`
}

func (l Launch) valid() bool {
	return l.Version == 2 && guestproto.ValidID(l.JobID) && guestproto.ValidID(l.DeviceID) && guestproto.ValidID(l.ManagementToken) && releasePattern.MatchString(l.Release) && l.CatalogVersion >= 1
}
func EncodeLaunch(l Launch) ([]byte, error) {
	if !l.valid() {
		return nil, ErrManifest
	}
	raw, err := json.Marshal(l)
	if err != nil || len(raw) > MaxDispatchBytes {
		return nil, ErrManifest
	}
	return raw, nil
}
func DecodeLaunch(raw []byte) (Launch, error) {
	if len(raw) > MaxDispatchBytes || !utf8.Valid(raw) {
		return Launch{}, ErrManifest
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return Launch{}, ErrManifest
	}
	seen := map[string]bool{}
	for decoder.More() {
		key, err := decoder.Token()
		name, ok := key.(string)
		if err != nil || !ok || seen[name] {
			return Launch{}, ErrManifest
		}
		switch name {
		case "version", "jobId", "deviceId", "managementToken", "release", "catalogVersion":
		default:
			return Launch{}, ErrManifest
		}
		seen[name] = true
		var value json.RawMessage
		if decoder.Decode(&value) != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return Launch{}, ErrManifest
		}
	}
	end, err := decoder.Token()
	var launch Launch
	if err != nil || end != json.Delim('}') || len(seen) != 6 || decoder.Decode(new(any)) != io.EOF || json.Unmarshal(raw, &launch) != nil || !launch.valid() {
		return Launch{}, ErrManifest
	}
	return launch, nil
}

// AcquiredDispatch is called only after backup-owned runtime acquisition and
// management checkpoint. It binds the acquired root token to the original job.
func (l Launch) AcquiredDispatch(rootToken string) (Dispatch, error) {
	if !l.valid() || !guestproto.ValidID(rootToken) {
		return Dispatch{}, ErrManifest
	}
	dispatch := Dispatch{Version: 1, JobID: l.JobID, DeviceID: l.DeviceID, ManagementToken: l.ManagementToken, RuntimeToken: rootToken, Release: l.Release, CatalogVersion: l.CatalogVersion}
	if _, err := EncodeDispatch(dispatch); err != nil {
		return Dispatch{}, err
	}
	return dispatch, nil
}
