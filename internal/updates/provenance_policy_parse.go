package updates

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
)

// ParseProvenancePolicy parses protected configuration bytes. The caller must
// independently verify ownership and trust epoch before using the returned keys.
func ParseProvenancePolicy(data []byte) (ProvenancePolicy, error) {
	var zero ProvenancePolicy
	if len(data) > 16384 || !validEvidenceJSON(data) {
		return zero, ErrProvenanceBinding
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil {
		return zero, ErrProvenanceBinding
	}
	allowed := []string{"schema", "threshold", "keys", "builderId", "buildType", "externalParameters", "sourceUri", "sourceCommit"}
	if len(fields) != len(allowed) {
		return zero, ErrProvenanceBinding
	}
	for _, name := range allowed {
		if _, ok := fields[name]; !ok {
			return zero, ErrProvenanceBinding
		}
	}
	var record struct {
		Schema             int             `json:"schema"`
		Threshold          int             `json:"threshold"`
		Keys               []string        `json:"keys"`
		BuilderID          string          `json:"builderId"`
		BuildType          string          `json:"buildType"`
		ExternalParameters json.RawMessage `json:"externalParameters"`
		SourceURI          string          `json:"sourceUri"`
		SourceCommit       string          `json:"sourceCommit"`
	}
	if json.Unmarshal(data, &record) != nil || record.Schema != 1 || record.Threshold < 2 || record.Threshold > len(record.Keys) || len(record.Keys) > 16 || !validEvidenceJSON(record.ExternalParameters) {
		return zero, ErrProvenanceBinding
	}
	for _, value := range []string{record.BuilderID, record.BuildType, record.SourceURI} {
		if len(value) == 0 || len(value) > 2048 {
			return zero, ErrProvenanceBinding
		}
	}
	if len(record.SourceCommit) != 40 && len(record.SourceCommit) != 64 {
		return zero, ErrProvenanceBinding
	}
	commit, err := hex.DecodeString(record.SourceCommit)
	if err != nil || hex.EncodeToString(commit) != record.SourceCommit {
		return zero, ErrProvenanceBinding
	}
	keys := []ed25519.PublicKey{}
	seen := map[string]bool{}
	for _, encoded := range record.Keys {
		if len(encoded) != 64 || seen[encoded] {
			return zero, ErrProvenanceBinding
		}
		key, err := hex.DecodeString(encoded)
		if err != nil || hex.EncodeToString(key) != encoded {
			return zero, ErrProvenanceBinding
		}
		seen[encoded] = true
		keys = append(keys, ed25519.PublicKey(key))
	}
	return ProvenancePolicy{Keys: keys, Threshold: record.Threshold, BuilderID: record.BuilderID, BuildType: record.BuildType, ExternalParameters: record.ExternalParameters, SourceURI: record.SourceURI, SourceCommit: record.SourceCommit}, nil
}
