package identity

import (
	"strings"
	"testing"
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
