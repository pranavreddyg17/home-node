package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func configurationRemoveFixture(t *testing.T) (*Engine, string, string) {
	t.Helper()
	host, jr := roots(t)
	e := openEngine(t, host, jr)
	plan := fixturePlan()
	plan.Items = append(plan.Items, Item{Path: "etc/homenode/services.env", Mode: 0640, UID: os.Geteuid(), GID: os.Getegid(), Data: []byte("owned environment")})
	if err := e.Apply(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if _, err := e.prepareConfigurationRemovalLocked(context.Background()); err != nil {
		t.Fatal(err)
	}
	return e, host, jr
}

func TestConfigurationRemovalResumesUnlinkWithoutDeletingData(t *testing.T) {
	e, host, jr := configurationRemoveFixture(t)
	ctx := context.Background()
	dataPath := filepath.Join(host, "var/lib/homenode/control/management.db")
	if err := os.WriteFile(dataPath, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	stopped := errors.New("lost unlink acknowledgement")
	e.checkpoint = func(stage, name string) error {
		if stage == "configuration-removed" {
			return stopped
		}
		return nil
	}
	guard := func(context.Context) error { return nil }
	if err := e.removeConfigurationLocked(ctx, guard); !errors.Is(err, stopped) {
		t.Fatal(err)
	}
	e.Close()
	e = openEngine(t, host, jr)
	defer e.Close()
	if err := e.removeConfigurationLocked(ctx, guard); err != nil {
		t.Fatal(err)
	}
	if err := e.removeConfigurationLocked(ctx, guard); err != nil {
		t.Fatal("ready replay", err)
	}
	data, err := os.ReadFile(dataPath)
	if err != nil || string(data) != "preserve" {
		t.Fatal("data removed", err)
	}
	installed, err := e.load()
	if err != nil {
		t.Fatal(err)
	}
	intent, err := e.loadConfigurationRemovalIntent(installed)
	if err != nil || !intent.Ready || intent.Completed != 2 {
		t.Fatal(intent, err)
	}
}

func TestConfigurationRemovalRefusesMissingFutureFileAndReintroducedCompletedFile(t *testing.T) {
	for _, scenario := range []string{"future-missing", "reintroduced", "foreign"} {
		t.Run(scenario, func(t *testing.T) {
			e, host, _ := configurationRemoveFixture(t)
			defer e.Close()
			ctx := context.Background()
			guard := func(context.Context) error { return nil }
			policy := filepath.Join(host, "etc/homenode/runtime-policy.json")
			env := filepath.Join(host, "etc/homenode/services.env")
			switch scenario {
			case "future-missing":
				if err := os.Remove(env); err != nil {
					t.Fatal(err)
				}
			case "foreign":
				if err := os.WriteFile(env, []byte("foreign"), 0640); err != nil {
					t.Fatal(err)
				}
			case "reintroduced":
				if err := e.removeConfigurationLocked(ctx, guard); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(policy, []byte("fixture policy bytes"), 0640); err != nil {
					t.Fatal(err)
				}
			}
			if err := e.removeConfigurationLocked(ctx, guard); err == nil {
				t.Fatal("conflicting transition admitted")
			}
			if _, err := os.Stat(policy); err != nil {
				t.Fatal("conflict removed policy", err)
			}
		})
	}
}
