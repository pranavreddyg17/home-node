package control

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
)

func TestBackupOutcomesRequireAdministratorAndPreserveMissingEvidence(t *testing.T) {
	for _, caps := range []string{"", `["files"]`, `["admin"]`} {
		s := testServer(t)
		token := ""
		if caps != "" {
			token = seedSession(t, s, caps)
		}
		response := request(s, "GET", "http://localhost:8787/api/v1/backups/outcomes", "", token, "")
		want := 200
		if caps == "" {
			want = 401
		} else if caps != `["admin"]` {
			want = 403
		}
		if response.Code != want {
			t.Fatal(caps, response.Code, response.Body.String())
		}
		if want == 200 {
			var body map[string]any
			if json.Unmarshal(response.Body.Bytes(), &body) != nil || body["schema"] != float64(1) || body["current"] != nil || body["lastPublished"] != nil || len(body) != 3 {
				t.Fatal("missing publication reported incorrectly", response.Body.String())
			}
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("publication status cached")
			}
		}
	}
}

func TestBackupOutcomesRefuseCorruptEvidenceWithoutDisclosure(t *testing.T) {
	s := testServer(t)
	token := seedSession(t, s, `["admin"]`)
	if err := s.Store.Transaction(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO settings(key,value) VALUES('host.backup-outcome.current',?)", "private malformed evidence")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	response := request(s, "GET", "http://localhost:8787/api/v1/backups/outcomes", "", token, "")
	if response.Code != 503 || strings.Contains(response.Body.String(), "private") || strings.Contains(response.Body.String(), "lastPublished") {
		t.Fatal("corrupt outcome disclosed or converted to absence", response.Code, response.Body.String())
	}
}

func TestBackupOutcomesKeepUnknownSeparateFromEarlierPublication(t *testing.T) {
	s := testServer(t)
	token := seedSession(t, s, `["admin"]`)
	current := `{"version":1,"jobId":"current-job-1234567890","status":"unknown","snapshotId":"","claimedAt":20,"publishedAt":0}`
	last := `{"version":1,"jobId":"previous-job-1234567890","status":"published","snapshotId":"` + strings.Repeat("a", 64) + `","claimedAt":10,"publishedAt":11}`
	if err := s.Store.Transaction(context.Background(), func(tx *sql.Tx) error {
		for key, value := range map[string]string{"host.backup-outcome.current": current, "host.backup-outcome.last-success": last} {
			if _, err := tx.Exec("INSERT INTO settings(key,value) VALUES(?,?)", key, value); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	response := request(s, "GET", "http://localhost:8787/api/v1/backups/outcomes", "", token, "")
	var body struct {
		Schema  int `json:"schema"`
		Current struct {
			Status     string `json:"status"`
			SnapshotID string `json:"snapshotId"`
		} `json:"current"`
		Last struct {
			Status     string `json:"status"`
			SnapshotID string `json:"snapshotId"`
		} `json:"lastPublished"`
	}
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &body) != nil || body.Current.Status != "unknown" || body.Current.SnapshotID != "" || body.Last.Status != "published" || body.Last.SnapshotID != strings.Repeat("a", 64) {
		t.Fatal("unknown publication conflated with earlier success", response.Code, response.Body.String())
	}
}
