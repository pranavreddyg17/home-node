package identity

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/pranavreddyg17/home-node/internal/state"
	"strings"
	"testing"
	"time"
)

func TestBackupApprovalBindsRegisteredRepositoryAndRequest(t *testing.T) {
	repository := strings.Repeat("a", 64)
	body := []byte(`{"repositoryId":"` + repository + `"}`)
	key := "backup-request-1234567890"
	resources, err := BackupApprovalResources(body, repository, key)
	if err != nil || len(resources) != 2 || resources[0] != repository {
		t.Fatal(resources, err)
	}
	changed, err := BackupApprovalResources(body, repository, key+"-changed")
	if err != nil || changed[1] == resources[1] {
		t.Fatal("request key not bound", err)
	}
	for _, invalid := range []string{`{}`, `null`, `{"repositoryId":null}`, `{"repositoryId":"foreign"}`, `{"RepositoryId":"` + repository + `"}`, `{"repositoryId":"` + repository + `","password":"secret"}`, `{"repositoryId":"` + repository + `","repositoryId":"` + repository + `"}`} {
		if _, err := BackupApprovalResources([]byte(invalid), repository, key); err == nil {
			t.Fatal("invalid body accepted", invalid)
		}
	}
	for _, invalid := range []string{"", strings.Repeat("A", 64), strings.Repeat("a", 63)} {
		if _, err := BackupApprovalResources([]byte(`{"repositoryId":"`+invalid+`"}`), invalid, key); err == nil {
			t.Fatal("invalid identity accepted")
		}
	}
	if _, err := BackupApprovalResources(body, repository, "short"); err == nil {
		t.Fatal("short key accepted")
	}
}

func TestBackupAdmissionRejectsChangedApprovalAuthority(t *testing.T) {
	for _, mutation := range []string{"repository", "request", "body", "policy", "session"} {
		t.Run(mutation, func(t *testing.T) {
			s := testService(t)
			actor, _ := testDevice(t, s, AllCapabilities)
			repository := strings.Repeat("a", 64)
			key := "backup-request-1234567890"
			body := []byte(`{"repositoryId":"` + repository + `"}`)
			resources, err := BackupApprovalResources(body, repository, key)
			if err != nil {
				t.Fatal(err)
			}
			binding, err := newApprovalBinding(actor, "backup.create", resources, body, 1, time.Now().Unix())
			if err != nil {
				t.Fatal(err)
			}
			grant := state.Random()
			payload, _ := json.Marshal(challenge{Binding: &binding, Issuer: actor.Device.ID})
			if _, err = s.Store.DB.Exec("INSERT INTO challenges(token_hash,kind,payload,epoch,expires_at) VALUES(?,'approval-grant',?,?,?)", state.Hash(grant), string(payload), actor.Epoch, binding.ExpiresAt); err != nil {
				t.Fatal(err)
			}
			policy := int64(1)
			switch mutation {
			case "repository":
				repository = strings.Repeat("b", 64)
				body = []byte(`{"repositoryId":"` + repository + `"}`)
			case "request":
				key += "-changed"
			case "body":
				body = append(body, byte(' '))
			case "policy":
				policy = 2
			case "session":
				actor.TokenHash = state.Hash("another-session")
			}
			token, job, err := s.AdmitBackupApproved(context.Background(), actor, grant, body, repository, key, policy)
			if !errors.Is(err, ErrDenied) || token != "" || job.ID != "" {
				t.Fatal("changed authority admitted", token, job, err)
			}
			if err := s.Store.Transaction(context.Background(), state.RequireAdmission); err != nil {
				t.Fatal("refusal closed admission", err)
			}
			var count int
			if err := s.Store.DB.QueryRow("SELECT count(*) FROM challenges WHERE token_hash=?", state.Hash(grant)).Scan(&count); err != nil || count != 1 {
				t.Fatal("refusal consumed grant", count, err)
			}
		})
	}
}
