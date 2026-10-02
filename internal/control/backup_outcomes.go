package control

import (
	"database/sql"
	"errors"
	"github.com/pranavreddyg17/home-node/internal/state"
	"net/http"
	"time"
)

// backupOutcomes reports durable publication evidence only. It grants no
// maintenance authority and makes no claim of repository or restore health.
func (s *Server) backupOutcomes(w http.ResponseWriter, r *http.Request) {
	days, err := s.Store.BackupReminderInterval(r.Context())
	if err != nil {
		writeJSON(w, 503, map[string]string{"error": "Backup reminder settings are unavailable."})
		return
	}
	observation, err := s.Store.InspectBackupObservation(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Backup publication status is unavailable."})
		return
	}
	resumeJobID := ""
	if observation.WorkerCompletion == "complete" && observation.Current != nil && s.config.Runtime != nil && s.backupTasks != nil {
		s.backupTasks.mu.Lock()
		available := !s.backupTasks.active && !s.backupTasks.stopping
		s.backupTasks.mu.Unlock()
		if available {
			qualificationErr := s.Store.Transaction(r.Context(), func(tx *sql.Tx) error {
				_, job, err := state.AuthorizeReleasedBackupRecoveryTx(tx, actor(r).Device.ID, observation.Current.JobID)
				if err == nil {
					resumeJobID = job.ID
				}
				return err
			})
			if qualificationErr != nil && !errors.Is(qualificationErr, state.ErrMaintenanceOwner) && !errors.Is(qualificationErr, state.ErrBackupPublication) {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Backup restoration status is unavailable."})
				return
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"reminder": publicationBackupReminder(time.Now().Unix(), observation.LastPublished, days), "resumeJobId": resumeJobID, "schema": 1, "current": observation.Current, "lastPublished": observation.LastPublished, "workerCompletion": observation.WorkerCompletion})
}
