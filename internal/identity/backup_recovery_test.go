package identity

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/pranavreddyg17/home-node/internal/state"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestBackupResumeApprovalBindsJobKeyAndEmptyBody(t *testing.T) {
	job := strings.Repeat("A", 24)
	key := "recovery-request-1234567890"
	resources, err := BackupRecoveryApprovalResources([]byte(`{}`), job, key)
	if err != nil || len(resources) != 2 || !slices.IsSorted(resources) {
		t.Fatal(resources, err)
	}
	for _, body := range []string{`null`, `[]`, `{} {}`, `{"password":"secret"}`, `{"jobId":"foreign"}`} {
		if _, err := BackupRecoveryApprovalResources([]byte(body), job, key); err == nil {
			t.Fatal("invalid resume body accepted", body)
		}
	}
	for _, id := range []string{"", "short", strings.Repeat("A", 65), strings.Repeat("A", 20) + "/"} {
		if _, err := BackupRecoveryApprovalResources([]byte(`{}`), id, key); err == nil {
			t.Fatal("invalid job accepted", id)
		}
	}
	if _, err := BackupRecoveryApprovalResources([]byte(`{}`), job, "short"); err == nil {
		t.Fatal("missing key binding")
	}
	changed, err := BackupRecoveryApprovalResources([]byte(`{}`), job, key+"changed")
	if err != nil || reflect.DeepEqual(resources, changed) {
		t.Fatal("request key not bound", changed, err)
	}
	changed, err = BackupRecoveryApprovalResources([]byte(`{}`), strings.Repeat("B", 24), key)
	if err != nil || reflect.DeepEqual(resources, changed) {
		t.Fatal("job not bound", changed, err)
	}
}

func TestBackupResumeGrantRollsBackUnqualifiedStateAndRefusesReplay(t *testing.T) {
	s := testService(t)
	actor, _ := testDevice(t, s, AllCapabilities)
	ctx := context.Background()
	token, job, err := s.Store.BeginMaintenanceJob(ctx, actor.Device.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Store.AdvanceMaintenanceJob(ctx, token, job.ID, "draining", "freezing"); err != nil {
		t.Fatal(err)
	}
	if err = s.Store.ClaimBackupLaunch(ctx, token, job.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.Store.AttachMaintenanceRoot(ctx, token, job.ID, state.Random()); err != nil {
		t.Fatal(err)
	}
	if err = s.Store.AdvanceMaintenanceJob(ctx, token, job.ID, "staging", "publishing"); err != nil {
		t.Fatal(err)
	}
	if err = s.Store.ClaimBackupPublication(ctx, token, job.ID, actor.Device.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.Store.RecordBackupPublished(ctx, token, job.ID, actor.Device.ID, state.Hash("snapshot")); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{}`)
	key := "resume-request-1234567890"
	resources, err := BackupRecoveryApprovalResources(body, job.ID, key)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := newApprovalBinding(actor, "backup.resume", resources, body, 1, time.Now().Unix())
	if err != nil {
		t.Fatal(err)
	}
	grant := state.Random()
	payload, _ := json.Marshal(challenge{Binding: &binding, Issuer: actor.Device.ID})
	if _, err = s.Store.DB.Exec("INSERT INTO challenges(token_hash,kind,payload,epoch,expires_at) VALUES(?,'approval-grant',?,?,?)", state.Hash(grant), string(payload), actor.Epoch, binding.ExpiresAt); err != nil {
		t.Fatal(err)
	}
	authority, owned, err := s.AuthorizeBackupResumeApproved(ctx, actor, grant, body, job.ID, key, 1)
	if err == nil || authority != "" || owned.ID != "" {
		t.Fatal("active worker exposed recovery authority", authority, owned, err)
	}
	var count int
	if err = s.Store.DB.QueryRow("SELECT count(*) FROM challenges WHERE token_hash=?", state.Hash(grant)).Scan(&count); err != nil || count != 1 {
		t.Fatal("refusal consumed grant", count, err)
	}
	if err = s.Store.RecordBackupWorkerCompleted(ctx, token, job.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.Store.AdvanceMaintenanceJob(ctx, token, job.ID, "publishing", "restoring"); err != nil {
		t.Fatal(err)
	}
	if err = s.Store.ReleaseMaintenanceRoot(ctx, token, job.ID, func(context.Context, string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []string{"key", "body", "policy", "session", "job"} {
		attempted, keyCopy, bodyCopy, policy, id := actor, key, body, int64(1), job.ID
		switch mutation {
		case "key":
			keyCopy += "changed"
		case "body":
			bodyCopy = []byte(`{} `)
		case "policy":
			policy = 2
		case "session":
			attempted.TokenHash = state.Hash("foreign-session")
		case "job":
			id = state.Random()
		}
		authority, owned, err = s.AuthorizeBackupResumeApproved(ctx, attempted, grant, bodyCopy, id, keyCopy, policy)
		if !errors.Is(err, ErrDenied) || authority != "" || owned.ID != "" {
			t.Fatal("changed recovery approval accepted", mutation, err)
		}
	}
	authority, owned, err = s.AuthorizeBackupResumeApproved(ctx, actor, grant, body, job.ID, key, 1)
	if err != nil || authority != token || owned.ID != job.ID || owned.RootToken != "" {
		t.Fatal("qualified grant not committed", owned, err)
	}
	authority, owned, err = s.AuthorizeBackupResumeApproved(ctx, actor, grant, body, job.ID, key, 1)
	if !errors.Is(err, ErrDenied) || authority != "" || owned.ID != "" {
		t.Fatal("resume grant replay exposed authority", err)
	}
}
