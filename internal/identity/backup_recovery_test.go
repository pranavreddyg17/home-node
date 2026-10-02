package identity

import (
	"reflect"
	"slices"
	"strings"
	"testing"
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
