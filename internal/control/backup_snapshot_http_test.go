package control

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestSnapshotSelectionCredentialEnvelopeIsExact(t *testing.T) {
	repository := state.Hash("repository")
	valid := `{"repositoryId":"` + repository + `","cursor":""}`
	for _, raw := range []string{valid, `{"RepositoryId":"` + repository + `","cursor":""}`, `{"repositoryId":"` + repository + `","cursor":null}`, `{"repositoryId":"` + repository + `","cursor":"","cursor":""}`, `{"repositoryId":"` + repository + `","cursor":"/host/path"}`, `{"repositoryId":"` + repository + `","cursor":"","command":"restore"}`, valid + `{}`, strings.Replace(valid, repository, state.Hash("foreign"), 1)} {
		data := make([]byte, 4)
		binary.BigEndian.PutUint32(data, uint32(len(raw)))
		data = append(data, raw...)
		data = append(data, []byte("fixture-secret")...)
		body, password, err := readBackupCredentialEnvelope(bytes.NewReader(data), func(raw []byte) error { _, err := snapshotSelectionCursor(raw, repository); return err })
		defer clear(password)
		if raw == valid {
			if err != nil || string(body) != raw || string(password) != "fixture-secret" {
				t.Fatal("valid envelope refused", err)
			}
		} else if err == nil || body != nil || password != nil {
			t.Fatal("invalid selector envelope admitted", raw, err)
		}
	}
	cursor := state.Hash("snapshot")
	if got, err := snapshotSelectionCursor([]byte(`{"repositoryId":"`+repository+`","cursor":"`+cursor+`"}`), repository); err != nil || got != cursor {
		t.Fatal("full cursor refused", got, err)
	}
}
