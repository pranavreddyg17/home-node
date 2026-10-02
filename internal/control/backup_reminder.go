package control

import (
	"github.com/pranavreddyg17/home-node/internal/state"
	"math"
)

const backupReminderIntervalSeconds = int64(7 * 24 * 60 * 60)

type backupReminder struct {
	State        string `json:"state"`
	IntervalDays int    `json:"intervalDays"`
	DueAt        int64  `json:"dueAt,omitempty"`
}

// Reminders use acknowledged publication, not repository/restore health.
// Invalid/future timestamps cannot make stale backups appear current.
func publicationBackupReminder(now int64, last *state.BackupOutcome) backupReminder {
	result := backupReminder{State: "unavailable", IntervalDays: 7}
	if now <= 0 {
		return result
	}
	if last == nil {
		result.State = "never-published"
		return result
	}
	if last.Status != "published" || last.PublishedAt <= 0 || last.PublishedAt > now || last.PublishedAt > math.MaxInt64-backupReminderIntervalSeconds {
		return result
	}
	result.DueAt = last.PublishedAt + backupReminderIntervalSeconds
	result.State = "current"
	if now >= result.DueAt {
		result.State = "overdue"
	}
	return result
}
