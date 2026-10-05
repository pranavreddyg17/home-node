package control

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestPreviewSelectionCredentialEnvelopeIsExact(t *testing.T) {
	repository := state.Hash("repository")
	valid := `{"repositoryId":"` + repository + `","snapshotId":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`
	for _, raw := range []string{valid, `{"RepositoryId":"` + repository + `","snapshotId":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`, `{"repositoryId":"` + repository + `","snapshotId":null}`, `{"repositoryId":"` + repository + `","snapshotId":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","snapshotId":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`, `{"repositoryId":"` + repository + `","snapshotId":"/host/path"}`, `{"repositoryId":"` + repository + `","snapshotId":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","command":"restore"}`, valid + `{}`, strings.Replace(valid, repository, state.Hash("foreign"), 1)} {
		data := make([]byte, 4)
		binary.BigEndian.PutUint32(data, uint32(len(raw)))
		data = append(data, raw...)
		data = append(data, []byte("fixture-secret")...)
		body, password, err := readBackupCredentialEnvelope(bytes.NewReader(data), func(raw []byte) error { _, err := snapshotPreviewSelection(raw, repository); return err })
		defer clear(password)
		if raw == valid {
			if err != nil || string(body) != raw || string(password) != "fixture-secret" {
				t.Fatal("valid envelope refused", err)
			}
		} else if err == nil || body != nil || password != nil {
			t.Fatal("invalid selector envelope admitted", raw, err)
		}
	}
	snapshotId := state.Hash("snapshot")
	if got, err := snapshotPreviewSelection([]byte(`{"repositoryId":"`+repository+`","snapshotId":"`+snapshotId+`"}`), repository); err != nil || got != snapshotId {
		t.Fatal("full snapshotId refused", got, err)
	}
}
