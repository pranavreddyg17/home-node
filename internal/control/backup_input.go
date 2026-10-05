package control

import (
	"bytes"
	"encoding/binary"
	"io"

	"github.com/pranavreddyg17/home-node/internal/backup"
	"github.com/pranavreddyg17/home-node/internal/identity"
)

const maxBackupCredentialRequest = 4 + 4096 + 8192

// readBackupCredentialRequest reads a big-endian request length, exact approved
// JSON bytes and raw password bytes. Credential bytes never enter an approval
// body, URL or header. Caller owns clearing the returned password. A fixed read
// buffer avoids leaving credential copies behind during io.ReadAll growth.
func readBackupCredentialRequest(reader io.Reader, repository, requestKey string) (body, password []byte, resultErr error) {
	return readBackupCredentialEnvelope(reader, func(request []byte) error {
		_, err := identity.BackupApprovalResources(request, repository, requestKey)
		return err
	})
}

func readBackupCredentialEnvelope(reader io.Reader, validate func([]byte) error) (body, password []byte, resultErr error) {
	raw := make([]byte, maxBackupCredentialRequest+1)
	defer clear(raw)
	n, err := io.ReadFull(reader, raw)
	if (err != nil && err != io.EOF && err != io.ErrUnexpectedEOF) || n < 5 || n > maxBackupCredentialRequest {
		return nil, nil, backup.ErrRepository
	}
	length := int(binary.BigEndian.Uint32(raw[:4]))
	if length < 1 || length > 4096 || length > n-5 {
		return nil, nil, backup.ErrRepository
	}
	request := raw[4 : 4+length]
	if validate == nil || validate(request) != nil {
		return nil, nil, backup.ErrRepository
	}
	secret := raw[4+length : n]
	if len(secret) < 1 || len(secret) > 8192 || bytes.ContainsAny(secret, "\x00\r\n") {
		return nil, nil, backup.ErrRepository
	}
	return bytes.Clone(request), bytes.Clone(secret), nil
}
