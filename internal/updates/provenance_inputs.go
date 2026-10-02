package updates

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"reflect"
)

// ValidateProvenanceInputs extends declared subject/builder binding with exact
// independently reviewed external parameters and a resolved source revision.
// Caller policy must be protected; this still does not authenticate the signer.
func ValidateProvenanceInputs(data []byte, packageSHA256, builderID, buildType string, externalParameters []byte, sourceURI, sourceCommit string) error {
	if len(sourceURI) == 0 || len(sourceURI) > 2048 || (len(sourceCommit) != 40 && len(sourceCommit) != 64) || !validEvidenceJSON(externalParameters) {
		return ErrProvenanceBinding
	}
	commit, err := hex.DecodeString(sourceCommit)
	if err != nil || hex.EncodeToString(commit) != sourceCommit {
		return ErrProvenanceBinding
	}
	if err := ValidateProvenanceBinding(data, packageSHA256, builderID, buildType); err != nil {
		return err
	}
	var statement map[string]json.RawMessage
	var predicate, definition map[string]json.RawMessage
	if json.Unmarshal(data, &statement) != nil || json.Unmarshal(statement["predicate"], &predicate) != nil || json.Unmarshal(predicate["buildDefinition"], &definition) != nil {
		return ErrProvenanceBinding
	}
	decode := func(raw []byte) (any, error) {
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		var value any
		err := decoder.Decode(&value)
		return value, err
	}
	expected, err := decode(externalParameters)
	if err != nil {
		return ErrProvenanceBinding
	}
	actual, err := decode(definition["externalParameters"])
	if err != nil || !reflect.DeepEqual(expected, actual) {
		return ErrProvenanceBinding
	}
	var dependencies []map[string]json.RawMessage
	if json.Unmarshal(definition["resolvedDependencies"], &dependencies) != nil || len(dependencies) == 0 || len(dependencies) > 256 {
		return ErrProvenanceBinding
	}
	matched := 0
	for _, dependency := range dependencies {
		var uri string
		if json.Unmarshal(dependency["uri"], &uri) != nil {
			continue
		}
		if uri != sourceURI {
			continue
		}
		var digest map[string]string
		if json.Unmarshal(dependency["digest"], &digest) != nil || digest["gitCommit"] != sourceCommit {
			return ErrProvenanceBinding
		}
		matched++
	}
	if matched != 1 {
		return ErrProvenanceBinding
	}
	return nil
}
