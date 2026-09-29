package guestproto

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestRejectsOversizedFrameBeforeAllocation(t *testing.T) {
	var wire bytes.Buffer
	_ = binary.Write(&wire, binary.BigEndian, uint32(MaxFrame+1))
	if err := Read(&wire, new(Request)); err == nil {
		t.Fatal("oversized frame accepted")
	}
}
func TestRejectsUnknownFields(t *testing.T) {
	var wire bytes.Buffer
	body := []byte(`{"version":1,"command":"sh"}`)
	_ = binary.Write(&wire, binary.BigEndian, uint32(len(body)))
	wire.Write(body)
	if err := Read(&wire, new(Request)); err == nil {
		t.Fatal("unknown command accepted")
	}
}
func FuzzRead(f *testing.F) {
	f.Add([]byte{0, 0, 0, 2, '{', '}'})
	f.Add([]byte{255, 255, 255, 255})
	f.Fuzz(func(t *testing.T, data []byte) { var r Request; _ = Read(bytes.NewReader(data), &r) })
}
func FuzzValidate(f *testing.F) {
	f.Add("../secret", "run", int64(-1))
	f.Fuzz(func(t *testing.T, id, op string, offset int64) {
		_ = Validate(Request{Version: 1, RequestID: id, ObjectID: id, Operation: op, Offset: offset})
	})
}
