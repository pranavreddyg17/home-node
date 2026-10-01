package control

import "net/http"

// backupOutcomes reports durable publication evidence only. It grants no
// maintenance authority and makes no claim of repository or restore health.
func (s *Server) backupOutcomes(w http.ResponseWriter, r *http.Request) {
	observation, err := s.Store.InspectBackupObservation(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Backup publication status is unavailable."})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"schema": 1, "current": observation.Current, "lastPublished": observation.LastPublished, "workerCompletion": observation.WorkerCompletion})
}
