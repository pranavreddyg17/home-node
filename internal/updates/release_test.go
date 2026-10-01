package updates

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/theupdateframework/go-tuf/v2/metadata"
)

func releaseMetadataFixture() ReleaseMetadata {
	return ReleaseMetadata{Schema: 1, Release: "0.1.0", Sequence: 5, Platform: "ubuntu-24.04-amd64", CatalogVersion: 3, MinimumSourceSchema: 3, MaximumSourceSchema: 4, ResultSchema: 4, SBOMTarget: "releases/0.1.0/sbom.json", ProvenanceTarget: "releases/0.1.0/provenance.json"}
}

func TestReleasePolicyCompatibilityAndSecurityFloor(t *testing.T) {
	policy := ReleasePolicy{MinimumSequence: 5, MinimumCatalogVersion: 3, CurrentStateSchema: 4}
	for _, scenario := range []string{"valid", "sequence", "catalog", "platform", "schema", "future-data", "old-data", "result-schema", "evidence-path", "same-evidence", "release-injection"} {
		t.Run(scenario, func(t *testing.T) {
			release := releaseMetadataFixture()
			switch scenario {
			case "sequence":
				release.Sequence = 4
			case "catalog":
				release.CatalogVersion = 2
			case "platform":
				release.Platform = "ubuntu-24.04-arm64"
			case "schema":
				release.Schema = 2
			case "future-data":
				release.MaximumSourceSchema = 3
			case "old-data":
				release.MinimumSourceSchema = 5
			case "result-schema":
				release.ResultSchema = 3
			case "evidence-path":
				release.SBOMTarget = "releases/../sbom.json"
			case "same-evidence":
				release.ProvenanceTarget = release.SBOMTarget
			case "release-injection":
				release.Release = "1.0\nExecStart=anything"
			}
			data, err := json.Marshal(release)
			if err != nil {
				t.Fatal(err)
			}
			custom := json.RawMessage(data)
			target := &metadata.TargetFiles{Path: "releases/0.1.0/homenode.deb", Custom: &custom}
			parsed, err := parseReleaseMetadata(target, policy)
			if scenario == "valid" {
				if err != nil || parsed != release {
					t.Fatal(parsed, err)
				}
			} else if err == nil || parsed != (ReleaseMetadata{}) {
				t.Fatal("incompatible release accepted", parsed, err)
			}
		})
	}
}

func TestReleaseMetadataRejectsAmbiguousAndUnknownAuthority(t *testing.T) {
	data, _ := json.Marshal(releaseMetadataFixture())
	for _, payload := range []string{
		strings.Replace(string(data), `"schema":1`, `"schema":1,"schema":1`, 1),
		strings.Replace(string(data), `"schema":1`, `"Schema":1`, 1),
		strings.Replace(string(data), `"schema":1`, `"schema":1,"\u0073chema":1`, 1),
		strings.Replace(string(data), `"schema":1`, `"schema":1,"newAuthority":true`, 1),
		string(data) + ` {}`,
		`null`,
		strings.Repeat(" ", 8193),
	} {
		custom := json.RawMessage(payload)
		if _, err := parseReleaseMetadata(&metadata.TargetFiles{Path: "homenode.deb", Custom: &custom}, ReleasePolicy{MinimumSequence: 1, MinimumCatalogVersion: 1, CurrentStateSchema: 4}); err == nil {
			t.Fatal("ambiguous release metadata accepted", payload)
		}
	}
}
