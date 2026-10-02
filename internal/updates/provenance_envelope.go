package updates

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// VerifyProvenanceEnvelope authenticates exact DSSE payload bytes with a
// threshold of distinct pinned Ed25519 keys. It does not establish builder
// identity policy, certificate issuer trust or complete release qualification.
func VerifyProvenanceEnvelope(data []byte, keys []ed25519.PublicKey, threshold int) ([]byte, error) {
	if threshold < 2 || threshold > len(keys) || len(keys) > 16 || !validEvidenceJSON(data) {
		return nil, ErrProvenanceBinding
	}
	seen := map[string]bool{}
	for _, key := range keys {
		if len(key) != ed25519.PublicKeySize || seen[string(key)] {
			return nil, ErrProvenanceBinding
		}
		seen[string(key)] = true
	}
	var envelope map[string]json.RawMessage
	if json.Unmarshal(data, &envelope) != nil {
		return nil, ErrProvenanceBinding
	}
	var payloadType, encoded string
	if json.Unmarshal(envelope["payloadType"], &payloadType) != nil || payloadType != "application/vnd.in-toto+json" || json.Unmarshal(envelope["payload"], &encoded) != nil {
		return nil, ErrProvenanceBinding
	}
	payload, err := decodeDSSEBase64(encoded)
	if err != nil || len(payload) > 4<<20 {
		return nil, ErrProvenanceBinding
	}
	var signatures []map[string]json.RawMessage
	if json.Unmarshal(envelope["signatures"], &signatures) != nil || len(signatures) == 0 || len(signatures) > 16 {
		return nil, ErrProvenanceBinding
	}
	pae := append([]byte(fmt.Sprintf("DSSEv1 %d %s %d ", len(payloadType), payloadType, len(payload))), payload...)
	accepted := map[string]bool{}
	for _, signature := range signatures {
		var encodedSignature string
		if json.Unmarshal(signature["sig"], &encodedSignature) != nil || len(encodedSignature) > 128 {
			return nil, ErrProvenanceBinding
		}
		decoded, err := decodeDSSEBase64(encodedSignature)
		if err != nil || len(decoded) != ed25519.SignatureSize {
			return nil, ErrProvenanceBinding
		}
		// keyid is an unauthenticated hint. Count only actual distinct trusted keys.
		for _, key := range keys {
			if ed25519.Verify(key, pae, decoded) {
				accepted[string(key)] = true
			}
		}
	}
	if len(accepted) < threshold || !validEvidenceJSON(payload) {
		return nil, ErrProvenanceBinding
	}
	return payload, nil
}

func decodeDSSEBase64(value string) ([]byte, error) {
	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.RawURLEncoding} {
		decoded, err := encoding.Strict().DecodeString(value)
		if err == nil && encoding.EncodeToString(decoded) == value {
			return decoded, nil
		}
	}
	return nil, ErrProvenanceBinding
}
