package install

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestConfigurationRemovalIntentPersistsExactProvenanceWithoutDeletion(t *testing.T) {
	host, jr := roots(t)
	e := openEngine(t, host, jr)
	ctx := context.Background()
	if err := e.Apply(ctx, fixturePlan()); err != nil {
		t.Fatal(err)
	}
	plan, err := e.prepareConfigurationRemovalLocked(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Files) != 1 {
		t.Fatal(plan)
	}
	info, err := os.Stat(filepath.Join(jr, "configuration-removal.json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal(info, err)
	}
	e.Close()
	e = openEngine(t, host, jr)
	defer e.Close()
	if _, err := e.prepareConfigurationRemovalLocked(ctx); err != nil {
		t.Fatal("replay", err)
	}
	installed, err := e.load()
	if err != nil {
		t.Fatal(err)
	}
	intent, err := e.loadConfigurationRemovalIntent(installed)
	if err != nil || intent.Ready || intent.Completed != 0 {
		t.Fatal(intent, err)
	}
	if _, err := os.Stat(filepath.Join(host, plan.Files[0].Path)); err != nil {
		t.Fatal("preparation deleted configuration", err)
	}
}

func TestConfigurationRemovalIntentRejectsPlanSubstitutionAndMalformedProgress(t *testing.T) {
	host, jr := roots(t)
	e := openEngine(t, host, jr)
	defer e.Close()
	ctx := context.Background()
	if err := e.Apply(ctx, fixturePlan()); err != nil {
		t.Fatal(err)
	}
	if _, err := e.prepareConfigurationRemovalLocked(ctx); err != nil {
		t.Fatal(err)
	}
	installed, err := e.load()
	if err != nil {
		t.Fatal(err)
	}
	original, err := e.loadConfigurationRemovalIntent(installed)
	if err != nil {
		t.Fatal(err)
	}
	mutations := []func(*configurationRemovalIntent){
		func(i *configurationRemovalIntent) { i.Plan.Files[0].Path = "etc/passwd" },
		func(i *configurationRemovalIntent) { i.Plan.Files[0].SHA256 = "foreign" },
		func(i *configurationRemovalIntent) { i.Plan.InstallationID = "foreign" },
		func(i *configurationRemovalIntent) { i.Plan.RetainedPaths = nil },
		func(i *configurationRemovalIntent) { i.Completed = 2 },
		func(i *configurationRemovalIntent) { i.Ready = true },
	}
	for index, mutate := range mutations {
		encoded, _ := json.Marshal(original)
		var changed configurationRemovalIntent
		if err := json.Unmarshal(encoded, &changed); err != nil {
			t.Fatal(err)
		}
		mutate(&changed)
		encoded, err = json.Marshal(changed)
		if err != nil {
			t.Fatal(err)
		}
		if err := e.journalRoot.WriteFile("configuration-removal.json", encoded, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := e.loadConfigurationRemovalIntent(installed); err == nil {
			t.Fatal("substituted plan admitted", index)
		}
	}
	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	encoded = append(encoded[:len(encoded)-1], []byte(",\"unknown\":true}")...)
	if err := e.journalRoot.WriteFile("configuration-removal.json", encoded, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := e.loadConfigurationRemovalIntent(installed); err == nil {
		t.Fatal("unknown field admitted")
	}
}
