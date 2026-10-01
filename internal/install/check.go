package install

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/pranavreddyg17/home-node/internal/catalog"
	"github.com/pranavreddyg17/home-node/internal/networkcheck"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
	servicetemplates "github.com/pranavreddyg17/home-node/packaging/systemd"
)

type InstallationCheck struct {
	Maintenance           *MaintenanceAccount `json:"maintenance,omitempty"`
	ConfigurationID       string              `json:"configurationId"`
	Network               networkcheck.Config `json:"network"`
	Accounts              Accounts            `json:"accounts"`
	RuntimePolicy         supervisor.Policy   `json:"runtimePolicy"`
	PublisherKeyID        string              `json:"publisherKeyId"`
	CatalogFloor          int64               `json:"catalogFloor"`
	CatalogVersion        int64               `json:"catalogVersion"`
	ArtifactsVerified     bool                `json:"artifactsVerified"`
	AccountsVerified      bool                `json:"accountsVerified"`
	HTTPSIdentityVerified bool                `json:"httpsIdentityVerified"`
	CertificateExpires    time.Time           `json:"certificateExpires"`
	Pending               []string            `json:"pending"`
}

// CheckInstallation is read only and derives its inputs from the committed
// owned installation. It cannot authorize service activation or prove peer ACLs.
func (e *Engine) CheckInstallation(ctx context.Context) (InstallationCheck, error) {
	var empty InstallationCheck
	if runtime.GOOS != "linux" || os.Geteuid() != 0 || e.host.Name() != "/" {
		return empty, ErrConflict
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	result, err := e.checkPrepared(ctx, time.Now())
	if err != nil {
		return empty, err
	}
	j, err := e.loadAccountJournal()
	if err != nil {
		return empty, err
	}
	snapshot, err := (nativeAccountProvisioner{}).Snapshot(ctx)
	defer clear(snapshot.shadow)
	if err != nil {
		return empty, err
	}
	for index := range creationCommands(j.OwnerID, j.Accounts) {
		present, err := accountStepMatches(snapshot, j, index)
		if err != nil || !present {
			return empty, ErrAccounts
		}
	}
	actual, err := InspectLocalAccounts(ctx)
	if err != nil {
		return empty, err
	}
	if actual != result.Accounts {
		return empty, ErrAccounts
	}
	if result.Maintenance != nil {
		observed, err := e.observeMaintenanceAccount(ctx, j)
		if err != nil || observed == nil || *observed != *result.Maintenance {
			return empty, ErrAccounts
		}
	}
	result.AccountsVerified = true
	if err = ctx.Err(); err != nil {
		return empty, err
	}
	if err = e.checkTLSAccess(result.Accounts); err != nil {
		return empty, err
	}
	certificate, err := networkcheck.Inspect(result.Network, "/etc/homenode/tls/server.crt", "/etc/homenode/tls/server.key")
	if err != nil {
		return empty, err
	}
	if err = ctx.Err(); err != nil {
		return empty, err
	}
	result.HTTPSIdentityVerified = true
	result.CertificateExpires = certificate.Leaf.NotAfter
	return result, nil
}

func (e *Engine) checkPrepared(ctx context.Context, now time.Time) (InstallationCheck, error) {
	var result InstallationCheck
	if err := ctx.Err(); err != nil {
		return result, err
	}
	config, err := e.load()
	if err != nil {
		return result, err
	}
	if config.Phase != "installed" {
		return result, ErrConflict
	}
	for _, r := range config.Items {
		if err = e.matches(r); err != nil {
			return result, ErrConflict
		}
	}
	a, err := e.loadAccountJournal()
	if err != nil {
		return result, err
	}
	if !a.Ready {
		return result, ErrAccounts
	}
	var maintenance *MaintenanceAccount
	unit, err := e.readConfiguration(config, "etc/systemd/system/homenode-supervisor.service")
	if err != nil {
		return result, err
	}
	standard, err := servicetemplates.Unit("homenode-supervisor.service")
	if err != nil {
		return result, err
	}
	if !bytes.Equal(unit, standard) {
		backup, err := e.loadMaintenanceAccountJournal(a)
		if err != nil || !backup.Ready {
			return result, ErrAccounts
		}
		expected, err := maintenanceUnit(standard, backup.Plan.Identity)
		if err != nil || !bytes.Equal(unit, expected) {
			return result, ErrPlan
		}
		identity := backup.Plan.Identity
		maintenance = &identity
	}
	if err = validateMaintenanceStaging(config, maintenance); err != nil {
		return result, err
	}
	controlUnit, err := e.readConfiguration(config, "etc/systemd/system/homenode-control.service")
	if err != nil {
		return result, err
	}
	expectedControl, err := servicetemplates.Unit("homenode-control.service")
	if err != nil {
		return result, err
	}
	if maintenance != nil {
		expectedControl, err = maintenanceControlUnit(expectedControl, *maintenance)
		if err != nil {
			return result, err
		}
		socketUnit, err := e.readConfiguration(config, "etc/systemd/system/homenode-app-maintenance.socket")
		if err != nil || !bytes.Equal(socketUnit, maintenanceSocketUnit()) {
			return result, ErrPlan
		}
		for _, name := range []string{"homenode-backup.service", "homenode-backup-credential.socket"} {
			actual, readErr := e.readConfiguration(config, "etc/systemd/system/"+name)
			expected, templateErr := servicetemplates.Unit(name)
			if readErr != nil || templateErr != nil || !bytes.Equal(actual, expected) {
				return result, ErrPlan
			}
		}
	}
	if !bytes.Equal(controlUnit, expectedControl) {
		return result, ErrPlan
	}
	policyBytes, err := e.readConfiguration(config, "etc/homenode/runtime-policy.json")
	if err != nil {
		return result, err
	}
	var policy supervisor.Policy
	d := json.NewDecoder(bytes.NewReader(policyBytes))
	d.DisallowUnknownFields()
	if d.Decode(&policy) != nil || d.Decode(new(any)) != io.EOF || policy.Validate() != nil || policy.ControllerUID != a.Accounts.ControllerUID || policy.TransferUID != a.Accounts.TransferUID {
		return result, ErrPlan
	}
	env, err := e.readConfiguration(config, "etc/homenode/services.env")
	if err != nil {
		return result, err
	}
	lines := strings.Split(string(env), "\n")
	if len(lines) != 8 || !strings.HasPrefix(lines[0], "TAILNET_IP=") || !strings.HasPrefix(lines[1], "HTTPS_PORT=") || !strings.HasPrefix(lines[2], "HTTPS_ORIGIN=") {
		return result, ErrPlan
	}
	port, err := strconv.Atoi(strings.TrimPrefix(lines[1], "HTTPS_PORT="))
	if err != nil {
		return result, ErrPlan
	}
	network := networkcheck.Config{Bind: strings.TrimPrefix(lines[0], "TAILNET_IP="), Port: port, Origin: strings.TrimPrefix(lines[2], "HTTPS_ORIGIN=")}
	if _, err = networkcheck.ValidateConfiguration(network); err != nil {
		return result, err
	}
	expected := fmt.Sprintf("TAILNET_IP=%s\nHTTPS_PORT=%d\nHTTPS_ORIGIN=%s\nPOLICY_GENERATION=%d\nCONTROLLER_UID=%d\nRUNTIME_GID=%d\nTRANSFER_GID=%d\n", network.Bind, network.Port, network.Origin, policy.Generation, a.Accounts.ControllerUID, a.Accounts.RuntimeGID, a.Accounts.TransferGID)
	if string(env) != expected {
		return result, ErrPlan
	}
	floorBytes, err := e.readConfiguration(config, "etc/homenode/catalog-floor")
	if err != nil {
		return result, err
	}
	floor, err := catalog.VersionFloor(floorBytes)
	if err != nil {
		return result, err
	}
	pubBytes, err := e.readConfiguration(config, "etc/homenode/catalog.pub")
	if err != nil {
		return result, err
	}
	pub, err := hex.DecodeString(strings.TrimSuffix(string(pubBytes), "\n"))
	if err != nil || len(pub) != ed25519.PublicKeySize || string(pubBytes) != hex.EncodeToString(pub)+"\n" {
		return result, ErrPlan
	}
	data, err := e.readConfiguration(config, "var/lib/homenode/catalog/catalog.json")
	if err != nil {
		return result, err
	}
	manifest, err := catalog.Verify(data, map[string]ed25519.PublicKey{catalog.KeyID(pub): pub}, floor, now)
	if err != nil || len(manifest.Images) != 3 {
		return result, catalog.ErrUntrusted
	}
	imageState, err := e.loadImageJournal()
	if err != nil {
		return result, err
	}
	if imageState.ConfigurationID != config.ID || imageState.CatalogDigest != digest(data) || imageState.Completed != len(manifest.Images) {
		return result, ErrConflict
	}
	images, err := protectedChildPath(e.host, "var/lib/homenode/images", e.owner, a.Accounts.QEMUGID)
	if err != nil {
		return result, err
	}
	defer images.Close()
	for _, image := range manifest.Images {
		if err = verifyPlacedImage(ctx, images, image.SHA256+".raw", image, e.owner, a.Accounts.QEMUGID); err != nil {
			return result, err
		}
	}
	result = InstallationCheck{Maintenance: maintenance, ConfigurationID: config.ID, Network: network, Accounts: a.Accounts, RuntimePolicy: policy, PublisherKeyID: catalog.KeyID(pub), CatalogFloor: floor, CatalogVersion: manifest.Version, ArtifactsVerified: true, Pending: []string{"verify supported host enforcement and measured VM overhead", "verify restrictive tailnet policy from allowed and denied devices", "validate and activate services", "complete passkey enrollment and phone sample job"}}
	if maintenance != nil {
		result.Pending = append(result.Pending, "register and qualify an external backup repository", "generate trusted backup launch configuration and qualify worker activation")
	}
	return result, nil
}

func (e *Engine) checkTLSAccess(a Accounts) error {
	for _, item := range []struct {
		name string
		mode os.FileMode
		gid  int
	}{
		{"etc/homenode/tls/server.crt", 0644, -1}, {"etc/homenode/tls/server.key", 0640, a.ControllerGID},
	} {
		info, err := e.host.Lstat(item.name)
		if err != nil || !info.Mode().IsRegular() || !owned(info, e.owner) || info.Mode().Perm() != item.mode {
			return ErrConflict
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok || (item.gid >= 0 && int(st.Gid) != item.gid) {
			return ErrConflict
		}
	}
	return nil
}
