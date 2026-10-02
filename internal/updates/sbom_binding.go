package updates

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
)

var ErrSBOMBinding = errors.New("release SBOM binding refused")

// ValidateSBOMBinding binds a CycloneDX 1.6 primary component to independently
// retained package identity. It does not validate the complete schema, inventory,
// licensing, vulnerabilities or installation authority.
func ValidateSBOMBinding(data []byte, packageSHA256, release string) error {
	if len(packageSHA256) != 64 || len(release) == 0 || len(release) > 64 || !validEvidenceJSON(data) {
		return ErrSBOMBinding
	}
	digest, err := hex.DecodeString(packageSHA256)
	if err != nil || hex.EncodeToString(digest) != packageSHA256 {
		return ErrSBOMBinding
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
	bom := object(data)
	if text(bom["bomFormat"]) != "CycloneDX" || text(bom["specVersion"]) != "1.6" {
		return ErrSBOMBinding
	}
	component := object(object(bom["metadata"])["component"])
	if text(component["type"]) != "application" || text(component["name"]) != "homenode" || text(component["version"]) != release {
		return ErrSBOMBinding
	}
	var hashes []json.RawMessage
	if json.Unmarshal(component["hashes"], &hashes) != nil || len(hashes) == 0 || len(hashes) > 16 {
		return ErrSBOMBinding
	}
	matched := 0
	for _, raw := range hashes {
		hash := object(raw)
		if text(hash["alg"]) == "SHA-256" {
			if text(hash["content"]) != packageSHA256 {
				return ErrSBOMBinding
			}
			matched++
		}
	}
	if matched != 1 {
		return ErrSBOMBinding
	}
	return nil
}
