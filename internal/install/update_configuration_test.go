package install

import (
	"bytes"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/updates"
	servicetemplates "github.com/pranavreddyg17/home-node/packaging/systemd"
	"github.com/sigstore/sigstore/pkg/signature"
	"github.com/theupdateframework/go-tuf/v2/metadata"
)

func updateBootstrapFixture(t *testing.T, threshold int) *updates.BootstrapRoot {
	t.Helper()
	root := metadata.Root(time.Now().UTC().Add(time.Hour))
	root.Signed.Roles[metadata.ROOT].Threshold = threshold
	var rootSigners []signature.Signer
	for _, role := range []string{metadata.ROOT, metadata.TIMESTAMP, metadata.SNAPSHOT, metadata.TARGETS} {
		count := 1
		if role == metadata.ROOT {
			count = threshold
		}
		for i := 0; i < count; i++ {
			public, private, err := ed25519.GenerateKey(rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			key, err := metadata.KeyFromPublicKey(public)
			if err != nil {
				t.Fatal(err)
			}
			if err = root.Signed.AddKey(key, role); err != nil {
				t.Fatal(err)
			}
			if role == metadata.ROOT {
				signer, err := signature.LoadSigner(private, crypto.Hash(0))
				if err != nil {
					t.Fatal(err)
				}
				rootSigners = append(rootSigners, signer)
			}
		}
	}
	for _, signer := range rootSigners {
		if _, err := root.Sign(signer); err != nil {
			t.Fatal(err)
		}
	}
	data, err := root.ToBytes(false)
	if err != nil {
		t.Fatal(err)
	}
	return &updates.BootstrapRoot{Data: data, SHA256: digest(data)}
}

func TestConfigurationProvisionsImmutableUpdateBootstrapAndPrivateCache(t *testing.T) {
	c, _, _, now := configurationFixture(t)
	c.UpdateBootstrap = updateBootstrapFixture(t, 2)
	c.Maintenance = &MaintenanceAccount{UID: 803, GID: 803}
	preview, err := ConfigurationPlan(c, now)
	if err != nil {
		t.Fatal(err)
	}
	if preview.UpdateBootstrapVersion != 1 || preview.UpdateBootstrapSHA256 != c.UpdateBootstrap.SHA256 {
		t.Fatal("bootstrap identity omitted")
	}
	bootstrap := findConfiguration(t, preview.Plan, "etc/homenode/update-root.json")
	if bootstrap.Mode != 0400 || bootstrap.UID != 0 || string(bootstrap.Data) != string(c.UpdateBootstrap.Data) {
		t.Fatal("unsafe immutable bootstrap", bootstrap)
	}
	for _, name := range []string{"var/lib/homenode-update", "var/lib/homenode-update/metadata", "var/lib/homenode-update/downloads"} {
		item := findConfiguration(t, preview.Plan, name)
		if !item.Directory || item.Mode != 0700 || item.UID != 0 || item.GID != 0 {
			t.Fatal("unsafe update directory", item)
		}
		if validRecord(record{Path: name, Directory: true, Mode: 0755, UID: 0, GID: 0}, 0) {
			t.Fatal("public update state accepted", name)
		}
	}
	for _, item := range preview.Plan.Items {
		if item.Path == "var/lib/homenode-update/metadata/root.json" {
			t.Fatal("mutable current root enrolled as immutable configuration")
		}
	}
}

func TestConfigurationRejectsUnpinnedAndSingleKeyUpdateRoots(t *testing.T) {
	for _, scenario := range []string{"pin-mismatch", "single-key", "invalid-root"} {
		t.Run(scenario, func(t *testing.T) {
			c, _, _, now := configurationFixture(t)
			c.UpdateBootstrap = updateBootstrapFixture(t, 2)
			switch scenario {
			case "pin-mismatch":
				c.UpdateBootstrap.SHA256 = digest([]byte("different root"))
			case "single-key":
				c.UpdateBootstrap = updateBootstrapFixture(t, 1)
			case "invalid-root":
				c.UpdateBootstrap.Data = []byte("malformed")
				c.UpdateBootstrap.SHA256 = digest(c.UpdateBootstrap.Data)
			}
			if _, err := ConfigurationPlan(c, now); err == nil {
				t.Fatal("unsafe bootstrap accepted")
			}
		})
	}
}

func TestConfigurationOwnsRepositoryPolicyAndRequiresBootstrap(t *testing.T) {
	c, _, _, now := configurationFixture(t)
	c.UpdateBootstrap = updateBootstrapFixture(t, 2)
	c.Maintenance = &MaintenanceAccount{UID: 803, GID: 803}
	withoutRepository, err := ConfigurationPlan(c, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range withoutRepository.Plan.Items {
		if item.Path == "etc/systemd/system/homenode-inspect.service" {
			t.Fatal("inspection service provisioned without repository policy")
		}
	}
	c.UpdateRepository = &updates.RepositoryConfiguration{Schema: 1, MetadataURL: "https://updates.example/metadata/", TargetsURL: "https://updates.example/targets/", MinimumSequence: 1, MinimumCatalogVersion: c.MinimumCatalogVersion}
	preview, err := ConfigurationPlan(c, now)
	if err != nil {
		t.Fatal(err)
	}
	unit := findConfiguration(t, preview.Plan, "etc/systemd/system/homenode-inspect.service")
	reviewed, err := servicetemplates.Unit("homenode-inspect.service")
	if err != nil || !bytes.Equal(unit.Data, reviewed) || unit.Mode != 0644 || unit.UID != 0 || unit.GID != 0 || unit.Directory {
		t.Fatal("inspection service ownership differs from reviewed source", err)
	}
	if bytes.Contains(unit.Data, []byte("[Install]")) {
		t.Fatal("inspection unit gains automatic enablement")
	}
	staging := findConfiguration(t, preview.Plan, "var/lib/homenode-update/inspection")
	if !staging.Directory || staging.Mode != 0700 || staging.UID != 0 || staging.GID != 0 {
		t.Fatal("unsafe inspection staging", staging)
	}
	if validRecord(record{Path: staging.Path, Directory: true, Mode: 0755, UID: 0, GID: 0}, 0) {
		t.Fatal("public inspection staging accepted")
	}
	item := findConfiguration(t, preview.Plan, "etc/homenode/update-repository.json")
	if item.Mode != 0400 || item.UID != 0 || item.GID != 0 || item.Directory {
		t.Fatal("repository policy ownership", item)
	}
	c.UpdateBootstrap = nil
	if _, err = ConfigurationPlan(c, now); err == nil {
		t.Fatal("repository without bootstrap accepted")
	}
	c.UpdateBootstrap = updateBootstrapFixture(t, 2)
	c.UpdateRepository.MinimumCatalogVersion = 0
	if _, err = ConfigurationPlan(c, now); err == nil {
		t.Fatal("invalid repository floor accepted")
	}
}
