package install

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/catalog"
	"github.com/pranavreddyg17/home-node/internal/networkcheck"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
	servicetemplates "github.com/pranavreddyg17/home-node/packaging/systemd"
)

func configurationFixture(t *testing.T) (Configuration, catalog.Manifest, ed25519.PrivateKey, time.Time) {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	manifest := catalog.Manifest{Schema: 1, Version: 4, Expires: now.Add(time.Hour), Images: []catalog.Image{
		{ID: "files", SHA256: digest([]byte("files fixture")), Bytes: 1024, MemoryMiB: 512, VCPUs: 1, DataBytes: 16 * catalog.GiB, Protocol: 1, License: "fixture-license", Version: "1"},
		{ID: "video", SHA256: digest([]byte("video fixture")), Bytes: 1024, MemoryMiB: 1024, VCPUs: 1, DataBytes: 8 * catalog.GiB, Protocol: 1, License: "fixture-license", Version: "1"},
		{ID: "ai", SHA256: digest([]byte("ai fixture")), Bytes: 1024, MemoryMiB: 2048, VCPUs: 1, DataBytes: 16 * catalog.GiB, Protocol: 1, License: "fixture-license", Version: "1"},
	}}
	c := Configuration{Network: networkcheck.Config{Bind: "100.100.1.2", Port: 8787, Origin: "https://home.example.ts.net:8787"}, Accounts: Accounts{ControllerUID: 800, TransferUID: 801, ControllerGID: 800, TransferGID: 801, RuntimeGID: 802, QEMUGID: 64055}, Policy: supervisor.Policy{Generation: 7, MemoryMiB: 4096, VCPUs: 2, MaxInstances: 3, DiskReserveBytes: 4 * catalog.GiB, ControllerUID: 800, TransferUID: 801}, Publisher: pub, MinimumCatalogVersion: 4, Capacity: Capacity{MemoryBytes: 8 * uint64(catalog.GiB), LogicalCPUs: 4, FreeDiskBytes: 100 * uint64(catalog.GiB)}}
	c.Catalog = signConfigurationCatalog(t, manifest, pub, key)
	return c, manifest, key, now
}
func signConfigurationCatalog(t *testing.T, m catalog.Manifest, pub ed25519.PublicKey, key ed25519.PrivateKey) []byte {
	t.Helper()
	payload, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(catalog.Envelope{KeyID: catalog.KeyID(pub), Payload: payload, Signature: ed25519.Sign(key, payload)})
	if err != nil {
		t.Fatal(err)
	}
	return data
}
func findConfiguration(t *testing.T, p Plan, name string) Item {
	t.Helper()
	for _, item := range p.Items {
		if item.Path == name {
			return item
		}
	}
	t.Fatal("missing planned path", name)
	return Item{}
}
func TestConfigurationPlanBindsPolicyUnitsAndCatalog(t *testing.T) {
	c, _, _, now := configurationFixture(t)
	result, err := ConfigurationPlan(c, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Pending) == 0 || result.CatalogVersion != 4 || result.RequiredDiskBytes != 44*uint64(catalog.GiB)+3072 {
		t.Fatal("incorrect preview", result)
	}
	env := string(findConfiguration(t, result.Plan, "etc/homenode/services.env").Data)
	for _, value := range []string{"TAILNET_IP=100.100.1.2\n", "HTTPS_ORIGIN=https://home.example.ts.net:8787\n", "POLICY_GENERATION=7\n", "CONTROLLER_UID=800\n", "RUNTIME_GID=802\n", "TRANSFER_GID=801\n"} {
		if !strings.Contains(env, value) {
			t.Fatal("missing binding", value)
		}
	}
	slice := string(findConfiguration(t, result.Plan, "etc/systemd/system/homenode.slice").Data)
	if !strings.Contains(slice, "MemoryMax=4294967296\nCPUQuota=200%\nTasksMax=512\n") {
		t.Fatal("slice disagrees with policy", slice)
	}
	for _, name := range []string{"homenode-control.service", "homenode-supervisor.service", "homenode-transfer.service"} {
		expected, err := servicetemplates.Unit(name)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(expected, findConfiguration(t, result.Plan, "etc/systemd/system/"+name).Data) {
			t.Fatal("unreviewed service template", name)
		}
	}
	result2, err := ConfigurationPlan(c, now)
	if err != nil {
		t.Fatal(err)
	}
	_, hash, _ := planRecords(result.Plan, 0)
	_, hash2, _ := planRecords(result2.Plan, 0)
	if hash != hash2 {
		t.Fatal("configuration plan not deterministic")
	}
	preview, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(preview, c.Catalog) || bytes.Contains(preview, []byte(base64.StdEncoding.EncodeToString(c.Catalog))) || bytes.Contains(preview, []byte("ExecStart=")) {
		t.Fatal("preview disclosed file content")
	}
}
func TestConfigurationRejectsUnsafeIdentityResourcesAndTrust(t *testing.T) {
	mutations := []struct {
		name   string
		change func(*Configuration)
	}{
		{"policy identity mismatch", func(c *Configuration) { c.Policy.ControllerUID = 65532 }},
		{"shared private group", func(c *Configuration) { c.Accounts.TransferGID = c.Accounts.ControllerGID }},
		{"root identity", func(c *Configuration) { c.Accounts.TransferUID = 0 }},
		{"rollback", func(c *Configuration) { c.MinimumCatalogVersion = 5 }},
		{"missing trust floor", func(c *Configuration) { c.MinimumCatalogVersion = 0 }},
		{"tampered catalog", func(c *Configuration) { c.Catalog[len(c.Catalog)/2] ^= 1 }},
		{"unknown publisher", func(c *Configuration) { c.Publisher = make(ed25519.PublicKey, 32) }},
		{"no host memory reserve", func(c *Configuration) { c.Capacity.MemoryBytes = 4 * uint64(catalog.GiB) }},
		{"no host CPU reserve", func(c *Configuration) { c.Capacity.LogicalCPUs = 2 }},
		{"insufficient disk", func(c *Configuration) { c.Capacity.FreeDiskBytes = 40 * uint64(catalog.GiB) }},
		{"serial files-video impossible", func(c *Configuration) { c.Policy.MaxInstances = 1 }},
		{"competing CPU impossible", func(c *Configuration) { c.Policy.VCPUs = 1 }},
		{"origin environment injection", func(c *Configuration) { c.Network.Origin = "https://home.example.ts.net:8787\nCONTROLLER_UID=0" }},
	}
	for _, tc := range mutations {
		t.Run(tc.name, func(t *testing.T) {
			c, _, _, now := configurationFixture(t)
			tc.change(&c)
			if _, err := ConfigurationPlan(c, now); err == nil {
				t.Fatal("unsafe configuration accepted")
			}
		})
	}
	for _, kind := range []string{"files", "video"} {
		t.Run("undersized "+kind+" disk", func(t *testing.T) {
			c, m, key, now := configurationFixture(t)
			for i := range m.Images {
				if m.Images[i].ID == kind {
					m.Images[i].DataBytes = catalog.GiB
				}
			}
			c.Catalog = signConfigurationCatalog(t, m, c.Publisher, key)
			if _, err := ConfigurationPlan(c, now); err == nil {
				t.Fatal("disk below workflow needs accepted")
			}
		})
	}
	c, m, key, now := configurationFixture(t)
	m.Images = m.Images[:2]
	c.Catalog = signConfigurationCatalog(t, m, c.Publisher, key)
	if _, err := ConfigurationPlan(c, now); err == nil {
		t.Fatal("incomplete product catalog accepted")
	}
}
func TestRootGeneratedConfigurationAppliesThroughJournal(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root-only temporary fixture")
	}
	c, _, _, now := configurationFixture(t)
	result, err := ConfigurationPlan(c, now)
	if err != nil {
		t.Fatal(err)
	}
	host, jr := roots(t)
	engine := openEngine(t, host, jr)
	defer engine.Close()
	if err = engine.Apply(context.Background(), result.Plan); err != nil {
		t.Fatal(err)
	}
	var actual supervisor.Policy
	data, err := os.ReadFile(filepath.Join(host, "etc/homenode/runtime-policy.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &actual); err != nil || actual != c.Policy {
		t.Fatal("policy not installed", actual, err)
	}
	if err = engine.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
}
