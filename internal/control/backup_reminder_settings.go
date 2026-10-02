package control

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)

func (s *Server) backupReminderSettings(w http.ResponseWriter, r *http.Request) {
	raw, readErr := io.ReadAll(io.LimitReader(r.Body, 513))
	if readErr != nil || len(raw) > 512 {
		fail(w, 400, "INVALID_REQUEST", "The reminder request is malformed or too large.")
		return
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		fail(w, 400, "INVALID_REQUEST", "Provide one intervalDays between 1 and 90.")
		return
	}
	key, err := decoder.Token()
	var days int
	if err != nil || key != "intervalDays" || decoder.Decode(&days) != nil || days < 1 || days > 90 || decoder.More() {
		fail(w, 400, "INVALID_REQUEST", "Provide one intervalDays between 1 and 90.")
		return
	}
	end, err := decoder.Token()
	var trailing any
	if err != nil || end != json.Delim('}') || decoder.Decode(&trailing) != io.EOF {
		fail(w, 400, "INVALID_REQUEST", "The reminder request is malformed.")
		return
	}
	if err = s.Store.SetBackupReminderInterval(r.Context(), actor(r).Device.ID, days); err != nil {
		fail(w, 409, "CONFIGURATION_PAUSED", "Reminder settings cannot change while maintenance is active.")
		return
	}
	writeJSON(w, 200, map[string]int{"intervalDays": days})
}
