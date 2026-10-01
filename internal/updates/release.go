package updates

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"path"
	"regexp"

	"github.com/theupdateframework/go-tuf/v2/metadata"
)

var errReleasePolicy = errors.New("release target is incompatible or outside trusted policy")
var releaseName = regexp.MustCompile(`^[0-9][a-zA-Z0-9.+~-]{0,63}$`)
var evidenceName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/-]{0,191}\.json$`)

// ReleasePolicy is current trusted host policy, never supplied by a browser.
// Sequence is a separate security floor from a Debian version comparison.
type ReleasePolicy struct {
	MinimumSequence       int64
	MinimumCatalogVersion int64
	CurrentStateSchema    int
}

// ReleaseMetadata is custom metadata attached to the signed TUF package target.
// Its migration range declares compatibility, not a tested migration or install
// grant. Installation still needs the approved, journaled maintenance plan.
type ReleaseMetadata struct {
	Schema              int    `json:"schema"`
	Release             string `json:"release"`
	Sequence            int64  `json:"sequence"`
	Platform            string `json:"platform"`
	CatalogVersion      int64  `json:"catalogVersion"`
	MinimumSourceSchema int    `json:"minimumSourceSchema"`
	MaximumSourceSchema int    `json:"maximumSourceSchema"`
	ResultSchema        int    `json:"resultSchema"`
	SBOMTarget          string `json:"sbomTarget"`
	ProvenanceTarget    string `json:"provenanceTarget"`
}

func parseReleaseMetadata(target *metadata.TargetFiles, policy ReleasePolicy) (ReleaseMetadata, error) {
	var result ReleaseMetadata
	if target == nil || target.Custom == nil || len(*target.Custom) == 0 || len(*target.Custom) > 8192 || policy.MinimumSequence < 1 || policy.MinimumCatalogVersion < 1 || policy.CurrentStateSchema < 1 {
		return result, errReleasePolicy
	}
	data := []byte(*target.Custom)
	// Detect repeated decoded keys (including escaped aliases) before typed
	// decoding: encoding/json otherwise accepts the last occurrence.
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return result, errReleasePolicy
	}
	seen := map[string]bool{}
	allowed := map[string]bool{"schema": true, "release": true, "sequence": true, "platform": true, "catalogVersion": true, "minimumSourceSchema": true, "maximumSourceSchema": true, "resultSchema": true, "sbomTarget": true, "provenanceTarget": true}
	for decoder.More() {
		token, err = decoder.Token()
		name, ok := token.(string)
		if err != nil || !ok || !allowed[name] || seen[name] {
			return result, errReleasePolicy
		}
		seen[name] = true
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return result, errReleasePolicy
		}
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') {
		return result, errReleasePolicy
	}
	if decoder.Decode(new(any)) != io.EOF {
		return result, errReleasePolicy
	}
	decoder = json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&result) != nil {
		return ReleaseMetadata{}, errReleasePolicy
	}
	if result.Schema != 1 || !releaseName.MatchString(result.Release) || result.Sequence < policy.MinimumSequence || result.Platform != "ubuntu-24.04-amd64" || result.CatalogVersion < policy.MinimumCatalogVersion || result.MinimumSourceSchema < 1 || result.MaximumSourceSchema < result.MinimumSourceSchema || result.MaximumSourceSchema > 1024 || policy.CurrentStateSchema < result.MinimumSourceSchema || policy.CurrentStateSchema > result.MaximumSourceSchema || result.ResultSchema < result.MaximumSourceSchema || result.ResultSchema > 1024 {
		return ReleaseMetadata{}, errReleasePolicy
	}
	for _, name := range []string{result.SBOMTarget, result.ProvenanceTarget} {
		if !evidenceName.MatchString(name) || path.Clean(name) != name {
			return ReleaseMetadata{}, errReleasePolicy
		}
	}
	if result.SBOMTarget == result.ProvenanceTarget || result.SBOMTarget == target.Path || result.ProvenanceTarget == target.Path {
		return ReleaseMetadata{}, errReleasePolicy
	}
	return result, nil
}
