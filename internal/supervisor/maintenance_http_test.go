package supervisor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestMaintenanceHTTPSeparatesPeerAuthorityAndReplaysOwnership(t *testing.T) {
	m, _ := newManager(t)
	backupUID := uint32(1003)
	handler := m.MaintenanceHandler(backupUID)
	call := func(uid uint32, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", path, strings.NewReader(body))
		if uid != 0 {
			r = r.WithContext(context.WithValue(r.Context(), peerKey{}, uid))
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	job := state.Random()
	body := `{"version":1,"jobId":"` + job + `"}`
	for _, uid := range []uint32{0, 1001, 1002, 1004} {
		if w := call(uid, "/v1/maintenance/begin", body); w.Code != 403 {
			t.Fatal(uid, w.Code)
		}
	}
	for _, invalid := range []string{
		body + "{}", `null`, `{"version":1,"version":1,"jobId":"` + job + `"}`,
		`{"version":1,"jobId":"` + job + `","job\u0049d":"` + job + `"}`,
		`{"version":1,"jobId":"` + job + `","token":"extra"}`,
		`{"version":1,"jobId":null}`, `{"version":2,"jobId":"` + job + `"}`,
		`{"version":1,"jobId":"` + job + `","shell":"anything"}`, strings.Repeat(" ", 513),
	} {
		if w := call(backupUID, "/v1/maintenance/begin", invalid); w.Code != 400 {
			t.Fatal(invalid, w.Code)
		}
	}
	w := call(backupUID, "/v1/maintenance/begin", body)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var acquired struct {
		Version int    `json:"version"`
		Token   string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &acquired); err != nil || acquired.Version != 1 || acquired.Token == "" {
		t.Fatal(acquired, err)
	}
	repeated := call(backupUID, "/v1/maintenance/begin", body)
	if repeated.Code != 200 || repeated.Body.String() != w.Body.String() {
		t.Fatal("acquisition replay changed", repeated.Code, repeated.Body.String())
	}
	if w = call(backupUID, "/v1/runtime", body); w.Code != http.StatusNotFound {
		t.Fatal("backup runtime escalation", w.Code)
	}
	if w = call(backupUID, "/v1/maintenance/end", `{"version":1,"token":"`+state.Random()+`"}`); w.Code != 409 {
		t.Fatal("foreign release", w.Code)
	}
	release := `{"version":1,"token":"` + acquired.Token + `"}`
	for i := 0; i < 2; i++ {
		if w = call(backupUID, "/v1/maintenance/end", release); w.Code != 200 {
			t.Fatal("release replay", w.Code, w.Body.String())
		}
	}
	if w = call(backupUID, "/v1/maintenance/begin", body); w.Code != 409 {
		t.Fatal("released job reacquired", w.Code)
	}
}

func TestMaintenanceHTTPRefusesOverlappingPeerRoles(t *testing.T) {
	m, _ := newManager(t)
	for _, uid := range []uint32{0, m.Policy.ControllerUID, m.Policy.TransferUID} {
		r := httptest.NewRequest("POST", "/v1/maintenance/begin", strings.NewReader(`{"version":1,"jobId":"`+state.Random()+`"}`))
		r = r.WithContext(context.WithValue(r.Context(), peerKey{}, uid))
		w := httptest.NewRecorder()
		m.MaintenanceHandler(uid).ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal("unsafe role configuration accepted", uid, w.Code)
		}
	}
}
