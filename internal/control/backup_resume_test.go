package control

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/pranavreddyg17/home-node/internal/identity"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestBackupResumeRoutesRequireAdministratorAndExactBody(t *testing.T) {
	s := testServer(t)
	origin := "http://localhost:8787"
	id := state.Random()
	path := origin + "/api/v1/backups/" + id + "/resume"
	for _, suffix := range []string{"", "/approval"} {
		if got := request(s, "POST", path+suffix, `{}`, "", origin); got.Code != 401 {
			t.Fatal(got.Code)
		}
	}
	token := seedSession(t, s, `["admin"]`)
	if got := request(s, "POST", path, `{}`, token, origin); got.Code != 409 {
		t.Fatal(got.Code, got.Body.String())
	}
	if got := request(s, "POST", path+"/approval", `{"password":"secret"}`, token, origin); got.Code != 403 || strings.Contains(got.Body.String(), "secret") {
		t.Fatal(got.Code, got.Body.String())
	}
	actor, err := s.Identity.Authenticate(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if id, err := s.StartApprovedBackupResume(ctx, actor, "unissued", []byte(`{}`), id, "resume-request-1234567890"); !errors.Is(err, context.Canceled) || id != "" {
		t.Fatal(id, err)
	}
	if err := s.Store.Transaction(context.Background(), state.RequireAdmission); err != nil {
		t.Fatal("refusal changed admission", err)
	}
}

func TestApprovedBackupResumeHTTPCompletesReleasedEmptyWorkloadJob(t *testing.T) {
	s := testServer(t)
	s.config.Runtime = fileBackend{}
	s.config.PolicyGeneration = 1
	session := state.Random()
	device := state.Random()
	now := time.Now().Unix()
	if _, err := s.Store.DB.Exec("INSERT INTO devices(id,name,capabilities,created_at) VALUES(?,'owner','[\"admin\"]',?)", device, now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Store.DB.Exec("INSERT INTO sessions VALUES(?,?,1,?,?,?,?)", state.Hash(session), device, now, now, now, now+3600); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	actor, err := s.Identity.Authenticate(ctx, session)
	if err != nil {
		t.Fatal(err)
	}
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
	if err = s.Store.RecordBackupWorkerCompleted(ctx, token, job.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.Store.AdvanceMaintenanceJob(ctx, token, job.ID, "publishing", "restoring"); err != nil {
		t.Fatal(err)
	}
	if err = s.Store.ReleaseMaintenanceRoot(ctx, token, job.ID, func(context.Context, string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err = s.Store.AdvanceMaintenanceJob(ctx, token, job.ID, "restoring", "requires-action"); err != nil {
		t.Fatal(err)
	}
	statusResponse := request(s, "GET", "http://localhost:8787/api/v1/backups/outcomes", "", session, "")
	if statusResponse.Code != 200 || !strings.Contains(statusResponse.Body.String(), `"resumeJobId":"`+job.ID+`"`) || strings.Contains(statusResponse.Body.String(), token) {
		t.Fatal("eligible resume status invalid", statusResponse.Code, statusResponse.Body.String())
	}
	s.backupTasks.mu.Lock()
	s.backupTasks.active = true
	s.backupTasks.mu.Unlock()
	busyStatus := request(s, "GET", "http://localhost:8787/api/v1/backups/outcomes", "", session, "")
	if busyStatus.Code != 200 || !strings.Contains(busyStatus.Body.String(), `"resumeJobId":""`) {
		t.Fatal("active task offered recovery", busyStatus.Body.String())
	}
	s.backupTasks.mu.Lock()
	s.backupTasks.active = false
	s.backupTasks.mu.Unlock()
	body := `{}`
	key := "resume-request-1234567890"
	grant := state.Random()
	resources, err := identity.BackupRecoveryApprovalResources([]byte(body), job.ID, key)
	if err != nil {
		t.Fatal(err)
	}
	binding := identity.ApprovalBinding{Action: "backup.resume", Resources: resources, BodySHA256: state.Hash(body), PolicyGeneration: 1, Epoch: actor.Epoch, ExpiresAt: time.Now().Unix() + 120, DeviceID: actor.Device.ID, SessionHash: actor.TokenHash}
	payload, _ := json.Marshal(map[string]any{"binding": binding, "issuer": actor.Device.ID})
	if _, err = s.Store.DB.Exec("INSERT INTO challenges(token_hash,kind,payload,epoch,expires_at) VALUES(?,'approval-grant',?,?,?)", state.Hash(grant), string(payload), actor.Epoch, binding.ExpiresAt); err != nil {
		t.Fatal(err)
	}
	send := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "http://localhost:8787/api/v1/backups/"+job.ID+"/resume", strings.NewReader(body))
		requestContext, requestCancel := context.WithCancel(ctx)
		defer requestCancel()
		r = r.WithContext(requestContext)
		r.URL.Scheme = ""
		r.URL.Host = ""
		r.Header.Set("Origin", "http://localhost:8787")
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Idempotency-Key", key)
		r.Header.Set("X-Action-Approval", grant)
		r.AddCookie(&http.Cookie{Name: s.cookie, Value: session})
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	response := send()
	if response.Code != 202 || !strings.Contains(response.Body.String(), job.ID) || strings.Contains(response.Body.String(), token) {
		t.Fatal("public acknowledgement invalid", response.Code, response.Body.String())
	}
	deadline, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	for {
		if admissionErr := s.Store.Transaction(ctx, state.RequireAdmission); admissionErr == nil {
			break
		}
		select {
		case <-deadline.Done():
			t.Fatal("server-owned resume did not finish after request cancellation")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err = s.CloseBackupWork(deadline); err != nil {
		t.Fatal("resume task not joined", err)
	}
	if err = s.Store.Transaction(ctx, state.RequireAdmission); err != nil {
		t.Fatal("successful resume kept admission closed", err)
	}
	var count int
	if err = s.Store.DB.QueryRow("SELECT count(*) FROM challenges WHERE token_hash=?", state.Hash(grant)).Scan(&count); err != nil || count != 0 {
		t.Fatal("resume approval not consumed", count, err)
	}
	if replay := send(); replay.Code == 202 {
		t.Fatal("resume task replay accepted")
	}
}
