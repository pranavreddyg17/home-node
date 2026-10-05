package control

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/pranavreddyg17/home-node/internal/backup"
)

func snapshotSelectionCursor(raw []byte, repository string) (string, error) {
	if !utf8.Valid(raw) {
		return "", backup.ErrManifest
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return "", backup.ErrManifest
	}
	fields := map[string]string{}
	for decoder.More() {
		key, err := decoder.Token()
		name, ok := key.(string)
		if err != nil || !ok || (name != "repositoryId" && name != "cursor") {
			return "", backup.ErrManifest
		}
		if _, exists := fields[name]; exists {
			return "", backup.ErrManifest
		}
		var value json.RawMessage
		if decoder.Decode(&value) != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return "", backup.ErrManifest
		}
		var text string
		if json.Unmarshal(value, &text) != nil {
			return "", backup.ErrManifest
		}
		fields[name] = text
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim('}') || len(fields) != 2 || decoder.Decode(new(any)) != io.EOF || fields["repositoryId"] != repository {
		return "", backup.ErrManifest
	}
	sentinel := "snapshot-validation-identity"
	if _, err := backup.EncodeSnapshotPageRequest(backup.SnapshotPageRequest{Version: 4, Kind: "snapshot-page", RequestID: sentinel, DeviceID: sentinel, Cursor: fields["cursor"]}); err != nil {
		return "", err
	}
	return fields["cursor"], nil
}

func (s *Server) backupSnapshotPage(w http.ResponseWriter, r *http.Request) {
	execution := s.config.BackupExecution
	if execution == nil || execution.SnapshotPage == nil || s.config.BackupRepositoryID == "" {
		fail(w, 503, "BACKUP_UNAVAILABLE", "Snapshot listing has not been configured.")
		return
	}
	if r.URL.RawQuery != "" {
		fail(w, 400, "INVALID_REQUEST", "Send snapshot selection and credential in the request body.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	if err := s.Identity.VerifyBackupSnapshotSession(ctx, actor(r)); err != nil {
		s.authError(w, err)
		return
	}
	var cursor string
	_, password, err := readBackupCredentialEnvelope(r.Body, func(raw []byte) error {
		var err error
		cursor, err = snapshotSelectionCursor(raw, s.config.BackupRepositoryID)
		return err
	})
	if err != nil {
		fail(w, 400, "INVALID_REQUEST", "The snapshot request is malformed or too large.")
		return
	}
	defer clear(password)
	request, release, err := s.beginSnapshotRequest(ctx, actor(r), cursor)
	if err != nil {
		s.authError(w, err)
		return
	}
	defer release()
	operation, err := s.snapshotOperationContext(request)
	if err != nil {
		s.authError(w, err)
		return
	}
	credential, err := backup.CreateRepositoryPassword(password)
	clear(password)
	if err != nil {
		fail(w, 503, "BACKUP_UNAVAILABLE", "Snapshot listing is unavailable.")
		return
	}
	defer credential.Close()
	page, err := execution.SnapshotPage(operation, request, credential)
	closeErr := credential.Close()
	if err != nil || closeErr != nil {
		fail(w, 503, "BACKUP_UNAVAILABLE", "The backup repository could not be listed.")
		return
	}
	if err := s.verifySnapshotRequest(ctx, request); err != nil {
		s.authError(w, err)
		return
	}
	if _, err := backup.EncodeSnapshotPageResponse(request.RequestID, page); err != nil {
		fail(w, 503, "BACKUP_UNAVAILABLE", "The snapshot response could not be verified.")
		return
	}
	writeJSON(w, http.StatusOK, page)
}
