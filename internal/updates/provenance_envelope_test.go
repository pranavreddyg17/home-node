package updates

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestProvenanceEnvelopeRequiresDistinctTrustedSigners(t *testing.T) {
	first := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{1}, 32))
	second := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{2}, 32))
	keys := []ed25519.PublicKey{first.Public().(ed25519.PublicKey), second.Public().(ed25519.PublicKey)}
	payload := []byte(`{"statement":"fixture"}`)
	// Independent literal PAE guards the protocol prefix and byte-length framing.
	pae := []byte(`DSSEv1 28 application/vnd.in-toto+json 23 {"statement":"fixture"}`)
	envelope := func(signers ...ed25519.PrivateKey) []byte {
		signatures := []map[string]string{}
		for _, signer := range signers {
			signatures = append(signatures, map[string]string{"sig": base64.URLEncoding.EncodeToString(ed25519.Sign(signer, pae)), "keyid": "untrusted-hint"})
		}
		data, err := json.Marshal(map[string]any{"payloadType": "application/vnd.in-toto+json", "payload": base64.StdEncoding.EncodeToString(payload), "signatures": signatures})
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	if verified, err := VerifyProvenanceEnvelope(envelope(first, second), keys, 2); err != nil || !bytes.Equal(verified, payload) {
		t.Fatal("valid threshold envelope refused", err)
	}
	for _, data := range [][]byte{envelope(first), envelope(first, first), bytes.Replace(envelope(first, second), []byte("application/vnd.in-toto+json"), []byte("application/json"), 1)} {
		if verified, err := VerifyProvenanceEnvelope(data, keys, 2); err == nil || verified != nil {
			t.Fatal("untrusted envelope exposed payload", err)
		}
	}
	if verified, err := VerifyProvenanceEnvelope(envelope(first, first), []ed25519.PublicKey{keys[0], keys[0]}, 2); err == nil || verified != nil {
		t.Fatal("aliased trusted key met threshold")
	}
}
