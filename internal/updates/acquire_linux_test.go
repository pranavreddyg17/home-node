//go:build linux

package updates

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestAcquireReleaseRejectsInvalidConfigurationBeforeCacheMutation(t *testing.T) {
	for _, scenario := range []string{"policy", "schema", "target", "metadata", "targets", "host", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			root, err := os.OpenRoot(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			policy := ReleasePolicy{MinimumSequence: 1, MinimumCatalogVersion: 1, CurrentStateSchema: 4}
			metadataURL, targetsURL, target := "https://updates.example/metadata/", "https://updates.example/targets/", "releases/home.deb"
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch scenario {
			case "policy":
				policy.MinimumSequence = 0
			case "schema":
				policy.CurrentStateSchema = 1025
			case "target":
				target = "releases/../home.deb"
			case "metadata":
				metadataURL = "http://updates.example/metadata/"
			case "targets":
				targetsURL += "?override=1"
			case "host":
				targetsURL = "https://other.example/targets/"
			case "canceled":
				cancel()
			}
			result, err := AcquireRelease(ctx, root, root, metadataURL, targetsURL, target, policy)
			if err == nil || result != nil {
				t.Fatal("invalid configuration accepted")
			}
			if scenario == "canceled" && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			entries, err := os.ReadDir(root.Name())
			if err != nil || len(entries) != 0 {
				t.Fatal("invalid configuration mutated cache", err)
			}
		})
	}
}
