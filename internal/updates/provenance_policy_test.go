package updates

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestReleaseProvenanceCombinesAuthenticationAndReviewedClaims(t *testing.T) {
	first := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{3}, 32))
	second := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{4}, 32))
	hash, commit := strings.Repeat("ab", 32), strings.Repeat("cd", 20)
	policy := ProvenancePolicy{Keys: []ed25519.PublicKey{first.Public().(ed25519.PublicKey), second.Public().(ed25519.PublicKey)}, Threshold: 2, BuilderID: "builder", BuildType: "type", ExternalParameters: json.RawMessage(`{"ref":"v1"}`), SourceURI: "git+https://source.example@v1", SourceCommit: commit}
	payload := `{"_type":"https://in-toto.io/Statement/v1","subject":[{"digest":{"sha256":"` + hash + `"}}],"predicateType":"https://slsa.dev/provenance/v1","predicate":{"buildDefinition":{"buildType":"type","externalParameters":{"ref":"v1"},"resolvedDependencies":[{"uri":"git+https://source.example@v1","digest":{"gitCommit":"` + commit + `"}}]},"runDetails":{"builder":{"id":"builder"}}}}`
	envelope := func(body string, signers ...ed25519.PrivateKey) []byte {
		pae := []byte(fmt.Sprintf("DSSEv1 28 application/vnd.in-toto+json %d %s", len(body), body))
		signatures := []map[string]string{}
		for _, signer := range signers {
			signatures = append(signatures, map[string]string{"sig": base64.StdEncoding.EncodeToString(ed25519.Sign(signer, pae))})
		}
		result, err := json.Marshal(map[string]any{"payloadType": "application/vnd.in-toto+json", "payload": base64.StdEncoding.EncodeToString([]byte(body)), "signatures": signatures})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	if err := VerifyReleaseProvenance(envelope(payload, first, second), hash, policy); err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{
		envelope(payload, first),
		envelope(strings.Replace(payload, hash, strings.Repeat("ef", 32), 1), first, second),
		envelope(strings.Replace(payload, `"ref":"v1"`, `"ref":"main"`, 1), first, second),
		envelope(strings.Replace(payload, commit, strings.Repeat("ef", 20), 1), first, second),
	} {
		if VerifyReleaseProvenance(data, hash, policy) == nil {
			t.Fatal("authentication or build policy bypassed")
		}
	}
}
