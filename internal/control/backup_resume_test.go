package control

import (
	"context"
	"errors"
	"strings"
	"testing"

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
