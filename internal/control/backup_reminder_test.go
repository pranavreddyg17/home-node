package control

import (
	"github.com/pranavreddyg17/home-node/internal/state"
	"math"
	"testing"
)

func TestBackupReminderUsesPublicationAgeAndRejectsInvalidClockEvidence(t *testing.T) {
	for _, tc := range []struct {
		now, published int64
		status, want   string
	}{
		{100, 10, "published", "current"},
		{10 + backupReminderIntervalSeconds, 10, "published", "overdue"},
		{11 + backupReminderIntervalSeconds, 10, "published", "overdue"},
		{0, 10, "published", "unavailable"},
		{100, 0, "published", "unavailable"},
		{100, 101, "published", "unavailable"},
		{100, 10, "unknown", "unavailable"},
		{math.MaxInt64, math.MaxInt64 - 1, "published", "unavailable"},
	} {
		reminder := publicationBackupReminder(tc.now, &state.BackupOutcome{Status: tc.status, PublishedAt: tc.published})
		if reminder.State != tc.want || reminder.IntervalDays != 7 {
			t.Fatal(tc, reminder)
		}
		if tc.want == "unavailable" && reminder.DueAt != 0 {
			t.Fatal("invalid timestamp exposed deadline", reminder)
		}
	}
	if reminder := publicationBackupReminder(100, nil); reminder.State != "never-published" || reminder.DueAt != 0 {
		t.Fatal(reminder)
	}
}
