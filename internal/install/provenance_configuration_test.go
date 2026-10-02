package install

import (
	"bytes"
	"github.com/pranavreddyg17/home-node/internal/updates"
	"strings"
	"testing"
)

func TestConfigurationProvisionsPrivateProvenancePolicy(t *testing.T) {
	c, _, _, now := configurationFixture(t)
	c.UpdateProvenance = []byte(`{"schema":1,"threshold":2,"keys":["` + strings.Repeat("ab", 32) + `","` + strings.Repeat("cd", 32) + `"],"builderId":"fixture","buildType":"fixture","externalParameters":{},"sourceUri":"fixture","sourceCommit":"` + strings.Repeat("ef", 20) + `"}`)
	if _, err := ConfigurationPlan(c, now); err == nil {
		t.Fatal("provenance without repository/bootstrap admitted")
	}
	c.UpdateBootstrap = updateBootstrapFixture(t, 2)
	c.UpdateRepository = &updates.RepositoryConfiguration{Schema: 1, MetadataURL: "https://updates.example/metadata/", TargetsURL: "https://updates.example/targets/", MinimumSequence: 1, MinimumCatalogVersion: c.MinimumCatalogVersion}
	preview, err := ConfigurationPlan(c, now)
	if err != nil {
		t.Fatal(err)
	}
	item := findConfiguration(t, preview.Plan, "etc/homenode/update-provenance.json")
	if item.Mode != 0400 || item.UID != 0 || item.GID != 0 || item.Directory || !bytes.Equal(item.Data, c.UpdateProvenance) {
		t.Fatal("unsafe provenance policy ownership")
	}
	c.UpdateProvenance = []byte(`{"schema":1}`)
	if _, err := ConfigurationPlan(c, now); err == nil {
		t.Fatal("incomplete trust policy provisioned")
	}
}

func TestProvenancePolicyPlanRequiresParentBeforeFile(t *testing.T) {
	parent := Item{Path: "etc/homenode", Directory: true, Mode: 0755, UID: 0, GID: 0}
	file := Item{Path: "etc/homenode/update-provenance.json", Mode: 0400, UID: 0, GID: 0, Data: []byte("fixture")}
	if _, _, err := planRecords(Plan{Items: []Item{file, parent}}, 0); err == nil {
		t.Fatal("file before owned parent admitted")
	}
	if _, _, err := planRecords(Plan{Items: []Item{parent, file}}, 0); err != nil {
		t.Fatal("correct provenance ordering refused", err)
	}
}
