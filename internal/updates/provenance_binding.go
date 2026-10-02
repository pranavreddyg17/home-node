package updates

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
)

var ErrProvenanceBinding = errors.New("release provenance binding refused")

// ValidateProvenanceBinding checks subject and declared builder/build-type claims
// against independently retained policy. It does not authenticate the signer,
// verify external parameters, qualify a builder, or authorize installation.
func ValidateProvenanceBinding(data []byte, packageSHA256, builderID, buildType string) error {
	if len(packageSHA256) != 64 {
		return ErrProvenanceBinding
	}
	digest, err := hex.DecodeString(packageSHA256)
	if len(packageSHA256) != 64 || err != nil || len(digest) != 32 || hex.EncodeToString(digest) != packageSHA256 || len(builderID) == 0 || len(builderID) > 2048 || len(buildType) == 0 || len(buildType) > 2048 || !validEvidenceJSON(data) {
		return ErrProvenanceBinding
	}
	object := func(raw json.RawMessage) map[string]json.RawMessage {
		raw = bytes.TrimSpace(raw)
		var fields map[string]json.RawMessage
		if len(raw) == 0 || raw[0] != '{' || json.Unmarshal(raw, &fields) != nil {
			return nil
		}
		return fields
	}
	text := func(raw json.RawMessage) string {
		var value string
		if json.Unmarshal(raw, &value) != nil {
			return ""
		}
		return value
	}
	statement := object(data)
	if text(statement["_type"]) != "https://in-toto.io/Statement/v1" || text(statement["predicateType"]) != "https://slsa.dev/provenance/v1" {
		return ErrProvenanceBinding
	}
	var subjects []json.RawMessage
	if json.Unmarshal(statement["subject"], &subjects) != nil || len(subjects) == 0 || len(subjects) > 32 {
		return ErrProvenanceBinding
	}
	matched := 0
	for _, subject := range subjects {
		fields := object(subject)
		hashes := object(fields["digest"])
		if len(hashes) == 0 {
			return ErrProvenanceBinding
		}
		if text(hashes["sha256"]) == packageSHA256 {
			matched++
		}
	}
	predicate := object(statement["predicate"])
	definition := object(predicate["buildDefinition"])
	run := object(predicate["runDetails"])
	builder := object(run["builder"])
	if matched != 1 || text(definition["buildType"]) != buildType || text(builder["id"]) != builderID || object(definition["externalParameters"]) == nil {
		return ErrProvenanceBinding
	}
	return nil
}
