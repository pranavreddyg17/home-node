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

func snapshotPreviewSelection(raw []byte, repository string) (string, error) {
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
		if err != nil || !ok || (name != "repositoryId" && name != "snapshotId") {
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
	if _, err := backup.EncodeSnapshotPreviewRequest(backup.SnapshotPreviewRequest{Version: 5, Kind: "snapshot-preview", RequestID: sentinel, DeviceID: sentinel, SnapshotID: fields["snapshotId"]}); err != nil {
		return "", err
	}
	return fields["snapshotId"], nil
}

func (s *Server) backupSnapshotPreview(w http.ResponseWriter, r *http.Request) {
	execution := s.config.BackupExecution
	if execution == nil || execution.SnapshotPreview == nil || s.config.BackupRepositoryID == "" {
		fail(w, 503, "BACKUP_UNAVAILABLE", "Snapshot preview has not been configured.")
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
	var snapshotId string
	_, password, err := readBackupCredentialEnvelope(r.Body, func(raw []byte) error {
		var err error
		snapshotId, err = snapshotPreviewSelection(raw, s.config.BackupRepositoryID)
		return err
	})
	if err != nil {
		fail(w, 400, "INVALID_REQUEST", "The snapshot request is malformed or too large.")
		return
	}
	defer clear(password)
	request, release, err := s.beginSnapshotPreviewRequest(ctx, actor(r), snapshotId)
	if err != nil {
		s.authError(w, err)
		return
	}
	defer release()
	operation, err := s.snapshotPreviewOperationContext(request)
	if err != nil {
		s.authError(w, err)
		return
	}
	credential, err := backup.CreateRepositoryPassword(password)
	clear(password)
	if err != nil {
		fail(w, 503, "BACKUP_UNAVAILABLE", "Snapshot preview is unavailable.")
		return
	}
	defer credential.Close()
	page, err := execution.SnapshotPreview(operation, request, credential)
	closeErr := credential.Close()
	if err != nil || closeErr != nil {
		fail(w, 503, "BACKUP_UNAVAILABLE", "The backup repository could not be inspected.")
		return
	}
	if err := s.verifySnapshotPreviewRequest(ctx, request); err != nil {
		s.authError(w, err)
		return
	}
	if _, err := backup.EncodeSnapshotPreviewResponse(request.RequestID, page); err != nil || page.SnapshotID != request.SnapshotID {
		fail(w, 503, "BACKUP_UNAVAILABLE", "The snapshot response could not be verified.")
		return
	}
	writeJSON(w, http.StatusOK, page)
}
