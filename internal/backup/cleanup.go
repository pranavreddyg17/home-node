package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"io"
	"net"
	"os"
	"unicode/utf8"
)

// Cleanup requests backup-peer runtime release after acknowledged worker stop.
// Version3 is disjoint from launch/dispatch and accepts no paths or commands.
type Cleanup struct {
	Version         int    `json:"version"`
	JobID           string `json:"jobId"`
	DeviceID        string `json:"deviceId"`
	ManagementToken string `json:"managementToken"`
	RuntimeToken    string `json:"runtimeToken"`
}

func (c Cleanup) valid() bool {
	return c.Version == 3 && guestproto.ValidID(c.JobID) && guestproto.ValidID(c.DeviceID) && guestproto.ValidID(c.ManagementToken) && guestproto.ValidID(c.RuntimeToken)
}
func EncodeCleanup(c Cleanup) ([]byte, error) {
	if !c.valid() {
		return nil, ErrManifest
	}
	raw, err := json.Marshal(c)
	if err != nil || len(raw) > MaxDispatchBytes {
		return nil, ErrManifest
	}
	return raw, nil
}
func DecodeCleanup(raw []byte) (Cleanup, error) {
	if len(raw) > MaxDispatchBytes || !utf8.Valid(raw) {
		return Cleanup{}, ErrManifest
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return Cleanup{}, ErrManifest
	}
	seen := map[string]bool{}
	for decoder.More() {
		token, err := decoder.Token()
		name, ok := token.(string)
		if err != nil || !ok || seen[name] {
			return Cleanup{}, ErrManifest
		}
		switch name {
		case "version", "jobId", "deviceId", "managementToken", "runtimeToken":
		default:
			return Cleanup{}, ErrManifest
		}
		seen[name] = true
		var value json.RawMessage
		if decoder.Decode(&value) != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return Cleanup{}, ErrManifest
		}
	}
	end, err := decoder.Token()
	var cleanup Cleanup
	if err != nil || end != json.Delim('}') || len(seen) != 5 || decoder.Decode(new(any)) != io.EOF || json.Unmarshal(raw, &cleanup) != nil || !cleanup.valid() {
		return Cleanup{}, ErrManifest
	}
	return cleanup, nil
}

func SendActivatedCredentialCleanup(ctx context.Context, connection *net.UnixConn, cleanup Cleanup, credential *os.File) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	raw, err := EncodeCleanup(cleanup)
	if err != nil {
		return err
	}
	return sendCredentialPayload(ctx, connection, 0, raw, credential)
}
func ReceiveCredentialCleanup(ctx context.Context, connection *net.UnixConn, controllerUID uint32) (Cleanup, *os.File, error) {
	return receiveCredentialPayload(ctx, connection, controllerUID, DecodeCleanup)
}
