package updates

import (
	"crypto/ed25519"
	"encoding/json"
)

// ProvenancePolicy must come from independently protected release policy, never
// from the envelope's key hints or claims. Builder keys require separate trust
// provisioning/rotation; this struct does not establish their provenance.
type ProvenancePolicy struct {
	Keys               []ed25519.PublicKey
	Threshold          int
	BuilderID          string
	BuildType          string
	ExternalParameters json.RawMessage
	SourceURI          string
	SourceCommit       string
}

// VerifyReleaseProvenance checks policy against the exact authenticated payload.
// It is one release gate; SBOM/vulnerability/rollback/install gates remain required.
func VerifyReleaseProvenance(envelope []byte, packageSHA256 string, policy ProvenancePolicy) error {
	payload, err := VerifyProvenanceEnvelope(envelope, policy.Keys, policy.Threshold)
	if err != nil {
		return err
	}
	return ValidateProvenanceInputs(payload, packageSHA256, policy.BuilderID, policy.BuildType, policy.ExternalParameters, policy.SourceURI, policy.SourceCommit)
}
