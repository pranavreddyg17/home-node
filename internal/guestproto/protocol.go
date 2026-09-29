// Package guestproto defines a bounded framed protocol. Guests cannot select
// host paths, destinations, commands or resource limits through this protocol.
package guestproto

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"regexp"
)

const Version = 1
const ChunkSize = 256 << 10
const MaxFrame = 512 << 10

var ErrProtocol = errors.New("invalid guest protocol message")
var resourcePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{20,64}$`)

func ValidID(id string) bool { return resourcePattern.MatchString(id) }

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type Request struct {
	Messages  []Message `json:"messages,omitempty"`
	Version   int       `json:"version"`
	RequestID string    `json:"requestId"`
	Operation string    `json:"operation"`
	ObjectID  string    `json:"objectId,omitempty"`
	Offset    int64     `json:"offset,omitempty"`
	Size      int64     `json:"size,omitempty"`
	SHA256    string    `json:"sha256,omitempty"`
	Data      []byte    `json:"data,omitempty"`
	InputID   string    `json:"inputId,omitempty"`
	Preset    string    `json:"preset,omitempty"`
	Prompt    string    `json:"prompt,omitempty"`
}
type Response struct {
	Version   int    `json:"version"`
	RequestID string `json:"requestId"`
	Error     string `json:"error,omitempty"`
	State     string `json:"state,omitempty"`
	Offset    int64  `json:"offset,omitempty"`
	Size      int64  `json:"size,omitempty"`
	SHA256    string `json:"sha256,omitempty"`
	Data      []byte `json:"data,omitempty"`
	Text      string `json:"text,omitempty"`
}

func Read(r io.Reader, value any) error {
	var length uint32
	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return err
	}
	if length == 0 || length > MaxFrame {
		return ErrProtocol
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return ErrProtocol
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return ErrProtocol
	}
	return nil
}
func Write(w io.Writer, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(data) > MaxFrame {
		return ErrProtocol
	}
	frame := make([]byte, 4+len(data))
	binary.BigEndian.PutUint32(frame, uint32(len(data)))
	copy(frame[4:], data)
	for len(frame) > 0 {
		n, err := w.Write(frame)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		frame = frame[n:]
	}
	return nil
}
func Validate(r Request) error {
	if r.Version != Version || !ValidID(r.RequestID) || r.Offset < 0 || r.Size < 0 || r.Size > 512<<30 || len(r.Data) > ChunkSize || len(r.Prompt) > 32<<10 {
		return ErrProtocol
	}
	if r.Operation != "health" && !ValidID(r.ObjectID) {
		return ErrProtocol
	}
	switch r.Operation {
	case "health", "upload", "finalize", "download", "delete", "stat", "cancel", "result":
		return nil
	case "run":
		if !ValidID(r.InputID) || (r.Preset != "mp4-720p" && r.Preset != "mp4-1080p") {
			return ErrProtocol
		}
		return nil
	case "generate":
		if len(r.Messages) > 17 || len(r.Messages) > 0 && r.Prompt != "" {
			return ErrProtocol
		}
		total := 0
		for _, m := range r.Messages {
			if m.Role != "user" && m.Role != "assistant" {
				return ErrProtocol
			}
			total += len(m.Content)
		}
		if total > 16<<10 || len(r.Messages) > 0 && r.Messages[len(r.Messages)-1].Role != "user" {
			return ErrProtocol
		}
		if r.InputID != "" && (!ValidID(r.InputID) || r.Prompt != "" || len(r.Messages) > 0) {
			return ErrProtocol
		}
		if r.Prompt == "" && len(r.Messages) == 0 && r.InputID == "" {
			return ErrProtocol
		}
		return nil
	default:
		return ErrProtocol
	}
}
