package control

import (
	"context"
	"strings"
	"testing"
)

func TestBackupReminderSettingsStrictInputPersistenceAndMaintenance(t *testing.T) {
	s := testServer(t)
	origin := "http://localhost:8787"
	path := origin + "/api/v1/backups/reminder"
	if got := request(s, "POST", path, `{"intervalDays":14}`, "", origin); got.Code != 401 {
		t.Fatal(got.Code)
	}
	limited := testServer(t)
	limitedSession := seedSession(t, limited, `["files"]`)
	if got := request(limited, "POST", path, `{"intervalDays":14}`, limitedSession, origin); got.Code != 403 {
		t.Fatal("limited device changed reminders", got.Code)
	}
	session := seedBackupSession(t, s)
	for _, body := range []string{`{}`, `null`, `{"intervalDays":null}`, `{"intervalDays":0}`, `{"intervalDays":91}`, `{"intervalDays":7.5}`, `{"intervalDays":"7"}`, `{"IntervalDays":7}`, `{"intervalDays":7,"intervalDays":14}`, `{"intervalDays":7,"\u0069ntervalDays":14}`, `{"intervalDays":7,"password":"secret"}`, `{"intervalDays":7} {}`} {
		if got := request(s, "POST", path, body, session, origin); got.Code != 400 {
			t.Fatal(body, got.Code, got.Body.String())
		}
	}
	if got := request(s, "POST", path, `{"intervalDays":14}`+strings.Repeat(" ", 512)+"garbage", session, origin); got.Code != 400 {
		t.Fatal("truncated oversized body accepted", got.Code)
	}
	if got := request(s, "POST", path, `{"intervalDays":14}`, session, origin); got.Code != 200 {
		t.Fatal(got.Code, got.Body.String())
	}
	if days, err := s.Store.BackupReminderInterval(context.Background()); err != nil || days != 14 {
		t.Fatal(days, err)
	}
	actor, err := s.Identity.Authenticate(context.Background(), session)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.Store.BeginMaintenanceJob(context.Background(), actor.Device.ID); err != nil {
		t.Fatal(err)
	}
	if got := request(s, "POST", path, `{"intervalDays":30}`, session, origin); got.Code != 409 {
		t.Fatal(got.Code)
	}
	if days, err := s.Store.BackupReminderInterval(context.Background()); err != nil || days != 14 {
		t.Fatal("maintenance mutated reminder", days, err)
	}
	if _, err = s.Store.DB.Exec("UPDATE settings SET value='014' WHERE key='backup.reminder.interval-days'"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Store.BackupReminderInterval(context.Background()); err == nil {
		t.Fatal("corrupt reminder silently adopted")
	}
}
