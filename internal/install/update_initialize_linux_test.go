//go:build linux

package install

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeOwnedUpdateTrustInitialization(t *testing.T) {
	if os.Getenv("HOMENODE_UPDATE_INIT_INTEGRATION") != "1" || os.Geteuid() != 0 {
		t.Skip("requires disposable Linux root fixture")
	}
	host, journal := roots(t)
	bootstrap := updateBootstrapFixture(t, 2)
	repository := []byte(`{"schema":1,"metadataUrl":"https://updates.example/metadata/","targetsUrl":"https://updates.example/targets/","minimumSequence":5,"minimumCatalogVersion":3}`)
	plan := Plan{Items: []Item{
		{Path: "etc/homenode", Directory: true, Mode: 0755, UID: 0, GID: 0},
		{Path: "etc/homenode/update-repository.json", Mode: 0400, UID: 0, GID: 0, Data: repository},
		{Path: "etc/homenode/update-root.json", Mode: 0400, UID: 0, GID: 0, Data: bootstrap.Data},
		{Path: "var/lib/homenode-update", Directory: true, Mode: 0700, UID: 0, GID: 0},
		{Path: "var/lib/homenode-update/metadata", Directory: true, Mode: 0700, UID: 0, GID: 0},
		{Path: "var/lib/homenode-update/downloads", Directory: true, Mode: 0700, UID: 0, GID: 0},
	}}
	engine := openEngine(t, host, journal)
	ctx := context.Background()
	if err := engine.Apply(ctx, plan); err != nil {
		engine.Close()
		t.Fatal(err)
	}
	if release, err := engine.acquireUpdateReleaseOwned(ctx, "releases/home.deb", 0); err == nil || release != nil {
		t.Fatal("invalid observed schema acquired release")
	}
	if _, err := os.Lstat(filepath.Join(host, "var/lib/homenode-update/bootstrap")); !os.IsNotExist(err) {
		t.Fatal("invalid schema initialized trust", err)
	}
	if err := engine.initializeUpdateCacheOwned(ctx); err != nil {
		engine.Close()
		t.Fatal(err)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	engine = openEngine(t, host, journal)
	defer engine.Close()
	if err := engine.initializeUpdateCacheOwned(ctx); err != nil {
		t.Fatal("restart rejected owned initialization", err)
	}
	currentPath := filepath.Join(host, "var/lib/homenode-update/metadata/root.json")
	current, err := os.ReadFile(currentPath)
	if err != nil || string(current) != string(bootstrap.Data) {
		t.Fatal("bootstrap not initialized", err)
	}
	policy, err := engine.readUpdateRepositoryOwned(ctx)
	if err != nil || policy.MinimumSequence != 5 || policy.MinimumCatalogVersion != 3 {
		t.Fatal("owned policy unavailable after restart", policy, err)
	}
	repositoryPath := filepath.Join(host, "etc/homenode/update-repository.json")
	if err = os.WriteFile(repositoryPath, []byte("changed"), 0400); err != nil {
		t.Fatal(err)
	}
	if _, err = engine.readUpdateRepositoryOwned(ctx); err == nil {
		t.Fatal("modified repository policy accepted")
	}
	if err = os.WriteFile(repositoryPath, repository, 0400); err != nil {
		t.Fatal(err)
	}
	bootstrapPath := filepath.Join(host, "etc/homenode/update-root.json")
	if err = os.WriteFile(bootstrapPath, []byte("changed"), 0400); err != nil {
		t.Fatal(err)
	}
	if err = engine.initializeUpdateCacheOwned(ctx); err == nil {
		t.Fatal("changed installer bootstrap accepted")
	}
	retained, err := os.ReadFile(currentPath)
	if err != nil || string(retained) != string(current) {
		t.Fatal("changed bootstrap replaced cache trust", err)
	}
	if err = os.WriteFile(bootstrapPath, bootstrap.Data, 0400); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(currentPath); err != nil {
		t.Fatal(err)
	}
	if err = engine.initializeUpdateCacheOwned(ctx); err == nil {
		t.Fatal("missing completed root reset")
	}
	if _, err = os.Lstat(currentPath); !os.IsNotExist(err) {
		t.Fatal("current root silently recreated", err)
	}
}
