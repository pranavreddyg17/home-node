//go:build linux

package install

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/pranavreddyg17/home-node/internal/updates"
	servicetemplates "github.com/pranavreddyg17/home-node/packaging/systemd"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeOwnedUpdateTrustInitialization(t *testing.T) {
	if os.Getenv("HOMENODE_UPDATE_INIT_INTEGRATION") != "1" || os.Geteuid() != 0 {
		t.Skip("requires disposable Linux root fixture")
	}
	host, journal := roots(t)
	bootstrap := updateBootstrapFixture(t, 2)
	repository := []byte(`{"schema":1,"metadataUrl":"https://updates.example/metadata/","targetsUrl":"https://updates.example/targets/","minimumSequence":5,"minimumCatalogVersion":3}`)
	provenance := []byte(`{"schema":1,"threshold":2,"keys":["` + strings.Repeat("ab", 32) + `","` + strings.Repeat("cd", 32) + `"],"builderId":"fixture","buildType":"fixture","externalParameters":{},"sourceUri":"fixture","sourceCommit":"` + strings.Repeat("ef", 20) + `"}`)
	service, err := servicetemplates.Unit("homenode-inspect.service")
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{Items: []Item{
		{Path: "etc/systemd/system/homenode-inspect.service", Mode: 0644, UID: 0, GID: 0, Data: service},
		{Path: "etc/homenode", Directory: true, Mode: 0755, UID: 0, GID: 0},
		{Path: "etc/homenode/update-provenance.json", Mode: 0400, UID: 0, GID: 0, Data: provenance},
		{Path: "etc/homenode/update-repository.json", Mode: 0400, UID: 0, GID: 0, Data: repository},
		{Path: "etc/homenode/update-root.json", Mode: 0400, UID: 0, GID: 0, Data: bootstrap.Data},
		{Path: "var/lib/homenode-update", Directory: true, Mode: 0700, UID: 0, GID: 0},
		{Path: "var/lib/homenode-update/metadata", Directory: true, Mode: 0700, UID: 0, GID: 0},
		{Path: "var/lib/homenode-update/downloads", Directory: true, Mode: 0700, UID: 0, GID: 0},
		{Path: "var/lib/homenode-update/inspection", Directory: true, Mode: 0700, UID: 0, GID: 0},
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
	if policy, err := engine.readUpdateProvenancePolicyOwned(ctx); err != nil || policy.Threshold != 2 || len(policy.Keys) != 2 {
		t.Fatal("owned provenance policy refused", err)
	}
	provenancePath := filepath.Join(host, "etc/homenode/update-provenance.json")
	if err := os.Chmod(provenancePath, 0440); err != nil {
		t.Fatal(err)
	}
	if policy, err := engine.readUpdateProvenancePolicyOwned(ctx); err == nil || len(policy.Keys) != 0 {
		t.Fatal("public provenance policy exposed trust keys", err)
	}
	if err := os.Chmod(provenancePath, 0400); err != nil {
		t.Fatal(err)
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
	data := []byte("signed acquisition staging fixture")
	packagePath := filepath.Join(t.TempDir(), "acquired.deb")
	if err := os.WriteFile(packagePath, data, 0400); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(packagePath)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	sum := sha256.Sum256(data)
	release := &updates.AcquiredRelease{Package: file, PackageSHA256: hex.EncodeToString(sum[:]), PackageLength: int64(len(data)), Metadata: updates.ReleaseMetadata{Release: "0.1.0", Platform: "ubuntu-24.04-amd64", Sequence: 4, CatalogVersion: 3}}
	if err := engine.stageUpdateInspectionOwned(ctx, release, "inspection-fixture-000001"); err == nil {
		t.Fatal("release below repository floor staged")
	}
	stagePath := filepath.Join(host, "var/lib/homenode-update/inspection")
	entries, err := os.ReadDir(stagePath)
	if err != nil || len(entries) != 0 {
		t.Fatal("rejected release mutated staging", err)
	}
	release.Metadata.Sequence = 5
	if err := engine.stageUpdateInspectionOwned(ctx, release, "inspection-fixture-000001"); err != nil {
		t.Fatal("owned inspection staging failed", err)
	}
	published, err := os.ReadFile(filepath.Join(stagePath, "package.deb"))
	if err != nil || string(published) != string(data) {
		t.Fatal("staged package differs", err)
	}
	stage, err := engine.openUpdateInspectionOwned(ctx, release, "inspection-fixture-000001")
	if err != nil {
		t.Fatal("owned inspection admission failed", err)
	}
	if err := stage.Close(); err != nil {
		t.Fatal(err)
	}
	servicePath := filepath.Join(host, "etc/systemd/system/homenode-inspect.service")
	if err := os.WriteFile(servicePath, []byte("changed service"), 0644); err != nil {
		t.Fatal(err)
	}
	if other, err := engine.prepareUpdateInspectionLaunchOwned(ctx, release, "inspection-fixture-000001"); err == nil {
		other.Close()
		t.Fatal("changed service admitted launch")
	}
	if _, err := os.Lstat(filepath.Join(host, "var/lib/homenode-update/inspection.env")); !os.IsNotExist(err) {
		t.Fatal("changed service published launch inputs", err)
	}
	if err := os.WriteFile(servicePath, service, 0644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"service.d", "homenode-.service.d", "homenode-inspect.service.d"} {
		dropIn := filepath.Join(host, "etc/systemd/system", name)
		if err := os.Mkdir(dropIn, 0755); err != nil {
			t.Fatal(err)
		}
		if other, err := engine.prepareUpdateInspectionLaunchOwned(ctx, release, "inspection-fixture-000001"); err == nil {
			other.Close()
			t.Fatal("unreviewed systemd drop-in admitted launch", name)
		}
		if _, err := os.Lstat(filepath.Join(host, "var/lib/homenode-update/inspection.env")); !os.IsNotExist(err) {
			t.Fatal("drop-in refusal published launch", err)
		}
		if err := os.Remove(dropIn); err != nil {
			t.Fatal(err)
		}
	}
	launch, err := engine.prepareUpdateInspectionLaunchOwned(ctx, release, "inspection-fixture-000001")
	if err != nil {
		t.Fatal("owned launch preparation failed", err)
	}
	expectedEnvironment, err := launch.Environment()
	if err != nil {
		t.Fatal(err)
	}
	publishedEnvironment, err := os.ReadFile(filepath.Join(host, "var/lib/homenode-update/inspection.env"))
	if err != nil || string(publishedEnvironment) != string(expectedEnvironment) {
		t.Fatal("owned launch configuration differs", err)
	}
	if other, err := engine.openUpdateInspectionOwned(ctx, release, "inspection-fixture-000001"); err == nil {
		other.Close()
		t.Fatal("prepared launch lost execution lock")
	}
	updateParent, err := engine.host.OpenRoot("var/lib/homenode-update")
	if err != nil {
		t.Fatal(err)
	}
	defer updateParent.Close()
	epoch, err := updates.CaptureInspectionLaunchEpoch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := launch.PublishLaunchIntent(ctx, updateParent, epoch); err != nil {
		t.Fatal("owned boot-bound launch intent failed", err)
	}
	if err := launch.VerifyLaunchIntent(ctx, updateParent, epoch); err != nil {
		t.Fatal("owned launch intent verification failed", err)
	}
	if err := launch.Close(); err != nil {
		t.Fatal(err)
	}
	if err := launch.VerifyLaunchIntent(ctx, updateParent, epoch); err == nil {
		t.Fatal("closed admission retained launch authority")
	}
	execution := updates.InspectionExecution{Epoch: epoch, InvocationID: "0123456789abcdef0123456789abcdef"}
	if result, err := engine.collectAndPublishUpdateInspectionResultOwned(ctx, release, "inspection-fixture-000001", execution); err == nil || result != (updates.InspectionResult{}) {
		t.Fatal("collection accepted missing recorded execution", result, err)
	}
	for _, name := range []string{"inspection.result", "inspection.result.pending"} {
		if _, err := updateParent.Lstat(name); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("failed collection published result evidence", name, err)
		}
	}
	afterCollection, err := engine.openUpdateInspectionOwned(ctx, release, "inspection-fixture-000001")
	if err != nil {
		t.Fatal("refused collection leaked execution lock", err)
	}
	if err := afterCollection.Close(); err != nil {
		t.Fatal(err)
	}
	if result, err := engine.readRecordedUpdateInspectionResultOwned(ctx, release, "inspection-fixture-000001", execution); err == nil || result != (updates.InspectionResult{}) {
		t.Fatal("missing recorded execution/result accepted", result, err)
	}
	// Failed readback must release its stage lock for explicit repair/recovery.
	afterReadback, err := engine.openUpdateInspectionOwned(ctx, release, "inspection-fixture-000001")
	if err != nil {
		t.Fatal("refused readback leaked execution lock", err)
	}
	if err := afterReadback.Close(); err != nil {
		t.Fatal(err)
	}
	if result, err := engine.readRecordedUpdateInspectionResultOwned(ctx, release, "inspection-fixture-000002", execution); err == nil || result != (updates.InspectionResult{}) {
		t.Fatal("unrelated recorded operation accepted", result, err)
	}
	if result, err := engine.collectAndPublishUpdateInspectionResultOwned(ctx, release, "inspection-fixture-000002", execution); err == nil || result != (updates.InspectionResult{}) {
		t.Fatal("collection admitted unrelated operation", result, err)
	}
	result, err := engine.withUpdateInspectionResultOwned(ctx, release, "inspection-fixture-000001", execution, func(_ *updates.InspectionStage, _ context.Context, _ *os.Root, _ updates.InspectionExecution) (updates.InspectionResult, error) {
		if err := os.WriteFile(servicePath, []byte("changed during collection"), 0644); err != nil {
			t.Fatal(err)
		}
		return updates.InspectionResult{Schema: 1, ContentValid: true}, nil
	})
	if !errors.Is(err, ErrConflict) || result != (updates.InspectionResult{}) {
		t.Fatal("changed service during collection exposed result", result, err)
	}
	if err := os.WriteFile(servicePath, service, 0644); err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []string{"repository", "sequence", "catalog", "platform", "release", "hash", "length", "cancellation"} {
		collectionCtx, cancel := context.WithCancel(ctx)
		metadata, packageHash, packageLength := release.Metadata, release.PackageSHA256, release.PackageLength
		policyPath := filepath.Join(host, "etc/homenode/update-repository.json")
		result, err := engine.withUpdateInspectionResultOwned(collectionCtx, release, "inspection-fixture-000001", execution, func(_ *updates.InspectionStage, _ context.Context, _ *os.Root, _ updates.InspectionExecution) (updates.InspectionResult, error) {
			switch mutation {
			case "repository":
				if err := os.WriteFile(policyPath, []byte("changed during collection"), 0400); err != nil {
					t.Fatal(err)
				}
			case "sequence":
				release.Metadata.Sequence = 4
			case "catalog":
				release.Metadata.CatalogVersion = 2
			case "platform":
				release.Metadata.Platform = "unsupported"
			case "release":
				release.Metadata.Release = "0.2.0"
			case "hash":
				release.PackageSHA256 = "changed"
			case "length":
				release.PackageLength++
			case "cancellation":
				cancel()
			}
			return updates.InspectionResult{Schema: 1, ContentValid: true}, nil
		})
		cancel()
		want := ErrConflict
		if mutation == "cancellation" {
			want = context.Canceled
		}
		if !errors.Is(err, want) || result != (updates.InspectionResult{}) {
			t.Fatal("post-collection policy change exposed evidence", mutation, result, err)
		}
		release.Metadata, release.PackageSHA256, release.PackageLength = metadata, packageHash, packageLength
		if err := os.WriteFile(policyPath, repository, 0400); err != nil {
			t.Fatal(err)
		}
		reopened, err := engine.openUpdateInspectionOwned(ctx, release, "inspection-fixture-000001")
		if err != nil {
			t.Fatal("post-collection refusal leaked execution lock", mutation, err)
		}
		if err := reopened.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(servicePath, []byte("changed readback service"), 0644); err != nil {
		t.Fatal(err)
	}
	if result, err := engine.readRecordedUpdateInspectionResultOwned(ctx, release, "inspection-fixture-000001", execution); !errors.Is(err, ErrConflict) || result != (updates.InspectionResult{}) {
		t.Fatal("readback bypassed changed service ownership", result, err)
	}
	if result, err := engine.collectAndPublishUpdateInspectionResultOwned(ctx, release, "inspection-fixture-000001", execution); !errors.Is(err, ErrConflict) || result != (updates.InspectionResult{}) {
		t.Fatal("collection policy refusal: readback bypassed changed service ownership", result, err)
	}
	if err := os.WriteFile(servicePath, service, 0644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"service.d", "homenode-.service.d", "homenode-inspect.service.d"} {
		dropIn := filepath.Join(host, "etc/systemd/system", name)
		if err := os.Mkdir(dropIn, 0755); err != nil {
			t.Fatal(err)
		}
		if result, err := engine.readRecordedUpdateInspectionResultOwned(ctx, release, "inspection-fixture-000001", execution); !errors.Is(err, ErrConflict) || result != (updates.InspectionResult{}) {
			t.Fatal("readback bypassed service drop-in refusal", name, result, err)
		}
		if result, err := engine.collectAndPublishUpdateInspectionResultOwned(ctx, release, "inspection-fixture-000001", execution); !errors.Is(err, ErrConflict) || result != (updates.InspectionResult{}) {
			t.Fatal("collection bypassed service drop-in refusal", name, result, err)
		}
		if err := os.Remove(dropIn); err != nil {
			t.Fatal(err)
		}
	}
	afterPolicyRefusal, err := engine.openUpdateInspectionOwned(ctx, release, "inspection-fixture-000001")
	if err != nil {
		t.Fatal("policy refusal leaked readback stage lock", err)
	}
	if err := afterPolicyRefusal.Close(); err != nil {
		t.Fatal(err)
	}
	if other, err := engine.prepareUpdateInspectionLaunchOwned(ctx, release, "inspection-fixture-000001"); err == nil {
		other.Close()
		t.Fatal("existing launch silently replaced")
	}
	// Failed repeat preparation must release admission for explicit recovery.
	reopened, err := engine.openUpdateInspectionOwned(ctx, release, "inspection-fixture-000001")
	if err != nil {
		t.Fatal("failed preparation leaked lock", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
	if other, err := engine.openUpdateInspectionOwned(ctx, release, "inspection-fixture-000002"); err == nil {
		other.Close()
		t.Fatal("wrong inspection operation admitted")
	}
	release.Metadata.Sequence = 4
	if result, err := engine.readRecordedUpdateInspectionResultOwned(ctx, release, "inspection-fixture-000001", execution); !errors.Is(err, ErrConflict) || result != (updates.InspectionResult{}) {
		t.Fatal("readback bypassed release floor", result, err)
	}
	if result, err := engine.collectAndPublishUpdateInspectionResultOwned(ctx, release, "inspection-fixture-000001", execution); !errors.Is(err, ErrConflict) || result != (updates.InspectionResult{}) {
		t.Fatal("collection policy refusal: readback bypassed release floor", result, err)
	}
	if other, err := engine.openUpdateInspectionOwned(ctx, release, "inspection-fixture-000001"); err == nil {
		other.Close()
		t.Fatal("staged release below floor admitted")
	}
	release.Metadata.Sequence = 5
	if err := engine.stageUpdateInspectionOwned(ctx, release, "inspection-fixture-000002"); err == nil {
		t.Fatal("occupied inspection operation replaced")
	}
	repositoryPath := filepath.Join(host, "etc/homenode/update-repository.json")
	if err = os.WriteFile(repositoryPath, []byte("changed"), 0400); err != nil {
		t.Fatal(err)
	}
	if _, err = engine.readUpdateRepositoryOwned(ctx); err == nil {
		t.Fatal("modified repository policy accepted")
	}
	if result, err := engine.readRecordedUpdateInspectionResultOwned(ctx, release, "inspection-fixture-000001", execution); !errors.Is(err, ErrConflict) || result != (updates.InspectionResult{}) {
		t.Fatal("readback bypassed modified repository ownership", result, err)
	}
	if result, err := engine.collectAndPublishUpdateInspectionResultOwned(ctx, release, "inspection-fixture-000001", execution); !errors.Is(err, ErrConflict) || result != (updates.InspectionResult{}) {
		t.Fatal("collection policy refusal: readback bypassed modified repository ownership", result, err)
	}
	if other, err := engine.openUpdateInspectionOwned(ctx, release, "inspection-fixture-000001"); err == nil {
		other.Close()
		t.Fatal("changed owned configuration admitted inspection")
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
