package control

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"testing"
)

func backupInputFrame(body, password []byte) []byte {
	result := make([]byte, 4+len(body)+len(password))
	binary.BigEndian.PutUint32(result, uint32(len(body)))
	copy(result[4:], body)
	copy(result[4+len(body):], password)
	return result
}

type observedCredentialReader struct {
	data     []byte
	retained []byte
	failure  bool
}

func (r *observedCredentialReader) Read(output []byte) (int, error) {
	r.retained = output
	n := copy(output, r.data)
	r.data = r.data[n:]
	if r.failure {
		return n, errors.New("fixture read failure")
	}
	return n, io.EOF
}

func TestBackupCredentialFrameBoundsAndClearing(t *testing.T) {
	repository := strings.Repeat("a", 64)
	key := "backup-request-1234567890"
	body := []byte(`{"repositoryId":"` + repository + `"}`)
	frame := backupInputFrame(body, []byte("fixture-password"))
	reader := &observedCredentialReader{data: frame}
	got, secret, err := readBackupCredentialRequest(reader, repository, key)
	if err != nil || !bytes.Equal(got, body) || string(secret) != "fixture-password" {
		t.Fatal("valid frame refused", err)
	}
	clear(secret)
	for _, value := range reader.retained {
		if value != 0 {
			t.Fatal("read buffer retained credential")
		}
	}
	cases := [][]byte{nil, {0, 0, 0, 0}, backupInputFrame(body, nil), backupInputFrame(body, []byte("bad\nsecret")), backupInputFrame(body, bytes.Repeat([]byte{'s'}, 8193)), backupInputFrame([]byte(`{"repositoryId":"foreign"}`), []byte("secret")), bytes.Repeat([]byte{'x'}, maxBackupCredentialRequest+1)}
	badLength := bytes.Clone(frame)
	binary.BigEndian.PutUint32(badLength, ^uint32(0))
	cases = append(cases, badLength)
	for _, input := range cases {
		reader := &observedCredentialReader{data: input}
		got, secret, err := readBackupCredentialRequest(reader, repository, key)
		if err == nil || got != nil || secret != nil {
			t.Fatal("invalid frame exposed data")
		}
		for _, value := range reader.retained {
			if value != 0 {
				t.Fatal("refusal retained input")
			}
		}
	}
	reader = &observedCredentialReader{data: frame, failure: true}
	if got, secret, err := readBackupCredentialRequest(reader, repository, key); err == nil || got != nil || secret != nil {
		t.Fatal("read failure admitted")
	}
	for _, value := range reader.retained {
		if value != 0 {
			t.Fatal("failed read retained input")
		}
	}
}
