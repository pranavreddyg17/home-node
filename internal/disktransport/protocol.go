package disktransport

import (
	"bytes"
	"encoding/json"
	"io"
	"regexp"
	"unicode/utf8"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
)

var imageDigest = regexp.MustCompile(`^[a-f0-9]{64}$`)

type Request struct {
	Version    int    `json:"version"`
	Token      string `json:"token"`
	InstanceID string `json:"instanceId"`
}
type Metadata struct {
	Version     int    `json:"version"`
	InstanceID  string `json:"instanceId"`
	Workload    string `json:"workload"`
	Bytes       int64  `json:"bytes"`
	ImageSHA256 string `json:"imageSha256"`
	DataSchema  int    `json:"dataSchema"`
	Protocol    int    `json:"protocol"`
}

func (r Request) Validate() error {
	if r.Version != 1 || !guestproto.ValidID(r.Token) || !guestproto.ValidID(r.InstanceID) {
		return ErrPacket
	}
	return nil
}
func (m Metadata) Validate() error {
	if m.Version != 1 || !guestproto.ValidID(m.InstanceID) || (m.Workload != "files" && m.Workload != "ai") || m.Bytes < 16<<20 || m.Bytes > 512<<30 || !imageDigest.MatchString(m.ImageSHA256) || m.DataSchema != 1 || m.Protocol != 1 {
		return ErrPacket
	}
	return nil
}
func DecodeRequest(data []byte) (Request, error) {
	var r Request
	if err := decodeObject(data, []string{"version", "token", "instanceId"}, &r); err != nil {
		return Request{}, err
	}
	if err := r.Validate(); err != nil {
		return Request{}, err
	}
	return r, nil
}
func DecodeMetadata(data []byte) (Metadata, error) {
	var m Metadata
	if err := decodeObject(data, []string{"version", "instanceId", "workload", "bytes", "imageSha256", "dataSchema", "protocol"}, &m); err != nil {
		return Metadata{}, err
	}
	if err := m.Validate(); err != nil {
		return Metadata{}, err
	}
	return m, nil
}
func decodeObject(data []byte, required []string, result any) error {
	if len(data) == 0 || len(data) > MaxPacket || !utf8.Valid(data) {
		return ErrPacket
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return ErrPacket
	}
	allowed := map[string]bool{}
	for _, name := range required {
		allowed[name] = true
	}
	seen := map[string]bool{}
	for decoder.More() {
		token, err := decoder.Token()
		name, ok := token.(string)
		if err != nil || !ok || !allowed[name] || seen[name] {
			return ErrPacket
		}
		seen[name] = true
		var value json.RawMessage
		if decoder.Decode(&value) != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return ErrPacket
		}
	}
	token, err = decoder.Token()
	if err != nil || token != json.Delim('}') || decoder.Decode(new(any)) != io.EOF || len(seen) != len(required) {
		return ErrPacket
	}
	if json.Unmarshal(data, result) != nil {
		return ErrPacket
	}
	return nil
}
