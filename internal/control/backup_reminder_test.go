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
		reminder := publicationBackupReminder(tc.now, &state.BackupOutcome{Status: tc.status, PublishedAt: tc.published}, 7)
		if reminder.State != tc.want || reminder.IntervalDays != 7 {
			t.Fatal(tc, reminder)
		}
		if tc.want == "unavailable" && reminder.DueAt != 0 {
			t.Fatal("invalid timestamp exposed deadline", reminder)
		}
	}
	if reminder := publicationBackupReminder(100, nil, 7); reminder.State != "never-published" || reminder.DueAt != 0 {
		t.Fatal(reminder)
	}
}

func TestBackupReminderHonorsSelectedInterval(t *testing.T) {
	last := &state.BackupOutcome{Status: "published", PublishedAt: 10}
	reminder := publicationBackupReminder(10+7*86400, last, 14)
	if reminder.State != "current" || reminder.IntervalDays != 14 || reminder.DueAt != 10+14*86400 {
		t.Fatal(reminder)
	}
	if reminder = publicationBackupReminder(10+14*86400, last, 14); reminder.State != "overdue" {
		t.Fatal(reminder)
	}
	for _, days := range []int{0, 91} {
		if reminder = publicationBackupReminder(100, last, days); reminder.State != "unavailable" || reminder.DueAt != 0 {
			t.Fatal(reminder)
		}
	}
}
