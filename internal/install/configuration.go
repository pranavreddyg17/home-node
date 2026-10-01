package install

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/pranavreddyg17/home-node/internal/catalog"
	"github.com/pranavreddyg17/home-node/internal/networkcheck"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
	"github.com/pranavreddyg17/home-node/internal/workload"
	servicetemplates "github.com/pranavreddyg17/home-node/packaging/systemd"
)

type Accounts struct {
	ControllerUID uint32
	TransferUID   uint32
	ControllerGID int
	TransferGID   int
	RuntimeGID    int
	QEMUGID       int
}
type Capacity struct {
	MemoryBytes   uint64
	LogicalCPUs   int
	FreeDiskBytes uint64
}
type Configuration struct {
	Maintenance           *MaintenanceAccount
	Network               networkcheck.Config
	Accounts              Accounts
	Policy                supervisor.Policy
	Publisher             ed25519.PublicKey
	Catalog               []byte
	MinimumCatalogVersion int64
	Capacity              Capacity
}
type ConfigurationPreview struct {
	Maintenance           *MaintenanceAccount `json:"maintenance,omitempty"`
	Network               networkcheck.Config `json:"network"`
	RuntimePolicy         supervisor.Policy   `json:"runtimePolicy"`
	Accounts              Accounts            `json:"accounts"`
	ProvidedCapacity      Capacity            `json:"providedCapacity"`
	Plan                  Plan                `json:"plan"`
	CatalogVersion        int64               `json:"catalogVersion"`
	PublisherKeyID        string              `json:"publisherKeyId"`
	RequiredDiskBytes     uint64              `json:"requiredDiskBytes"`
	VerifiedImageBytes    uint64              `json:"verifiedImageBytes"`
	RequiredFreeDiskBytes uint64              `json:"requiredFreeDiskBytes"`
	Pending               []string            `json:"pending"`
}

// ConfigurationPlan is deterministic and has no side effects. The caller must
// supply independently trusted publisher identity, current account IDs and
// measured capacity. Catalog signatures do not prove guest qualification.
func ConfigurationPlan(c Configuration, now time.Time) (ConfigurationPreview, error) {
	return configurationPlan(c, now, 0)
}

func configurationPlan(c Configuration, now time.Time, imageCredit uint64) (ConfigurationPreview, error) {
	var result ConfigurationPreview
	if _, err := networkcheck.ValidateConfiguration(c.Network); err != nil {
		return result, err
	}
	if err := c.Policy.Validate(); err != nil {
		return result, err
	}
	a := c.Accounts
	if a.ControllerUID == 0 || a.TransferUID == 0 || a.ControllerUID == a.TransferUID || a.ControllerUID >= 1000 || a.TransferUID >= 1000 || c.Policy.ControllerUID != a.ControllerUID || c.Policy.TransferUID != a.TransferUID {
		return result, ErrPlan
	}
	if a.ControllerGID >= 1000 || a.TransferGID >= 1000 || a.RuntimeGID >= 1000 {
		return result, ErrPlan
	}
	seen := map[int]bool{}
	for _, gid := range []int{a.ControllerGID, a.TransferGID, a.RuntimeGID, a.QEMUGID} {
		if gid < 1 || gid > 1<<31-1 || seen[gid] {
			return result, ErrPlan
		}
		seen[gid] = true
	}
	if c.Maintenance != nil {
		b := *c.Maintenance
		if b.UID < 100 || b.UID >= 1000 || b.UID == a.ControllerUID || b.UID == a.TransferUID || b.GID < 100 || b.GID >= 1000 || seen[b.GID] {
			return result, ErrPlan
		}
	}
	if len(c.Publisher) != ed25519.PublicKeySize || c.MinimumCatalogVersion < 1 {
		return result, ErrPlan
	}
	m, err := catalog.Verify(c.Catalog, map[string]ed25519.PublicKey{catalog.KeyID(c.Publisher): c.Publisher}, c.MinimumCatalogVersion, now)
	if err != nil {
		return result, err
	}
	if len(m.Images) != 3 || c.Policy.MaxInstances < 2 {
		return result, ErrPlan
	}
	if uint64(c.Policy.MemoryMiB+2048)*(1<<20) > c.Capacity.MemoryBytes || c.Policy.VCPUs >= c.Capacity.LogicalCPUs {
		return result, supervisor.ErrCapacity
	}
	required := uint64(c.Policy.DiskReserveBytes)
	for _, image := range m.Images {
		if image.MemoryMiB > c.Policy.MemoryMiB || image.VCPUs > c.Policy.VCPUs {
			return result, supervisor.ErrCapacity
		}
		required += uint64(image.Bytes + image.DataBytes)
	}
	filesImage, err := m.Image("files")
	if err != nil {
		return result, err
	}
	videoImage, err := m.Image("video")
	if err != nil {
		return result, err
	}
	if filesImage.DataBytes < workload.StorageQuota+4*catalog.GiB || videoImage.DataBytes < workload.MaxFileBytes+workload.MaxJobOutputBytes+2*catalog.GiB {
		return result, supervisor.ErrCapacity
	}
	if filesImage.MemoryMiB+videoImage.MemoryMiB > c.Policy.MemoryMiB || filesImage.VCPUs+videoImage.VCPUs > c.Policy.VCPUs {
		return result, supervisor.ErrCapacity
	}
	if c.Policy.MaxInstances > 3 {
		required += uint64(videoImage.DataBytes) * uint64(c.Policy.MaxInstances-3)
	}
	if imageCredit > required {
		return result, ErrPlan
	}
	if required-imageCredit > c.Capacity.FreeDiskBytes {
		return result, supervisor.ErrCapacity
	}
	plan := Plan{}
	addDir := func(name string, mode uint32, uid, gid int) {
		plan.Items = append(plan.Items, Item{Path: name, Directory: true, Mode: mode, UID: uid, GID: gid})
	}
	addFile := func(name string, mode uint32, data []byte) {
		plan.Items = append(plan.Items, Item{Path: name, Mode: mode, UID: 0, GID: 0, Data: append([]byte(nil), data...)})
	}
	addDir("etc/homenode", 0755, 0, 0)
	addDir("etc/homenode/tls", 0750, 0, a.ControllerGID)
	addDir("var/lib/homenode", 0755, 0, 0)
	addDir("var/lib/homenode/control", 0700, int(a.ControllerUID), a.ControllerGID)
	addDir("var/lib/homenode/supervisor", 0700, 0, 0)
	addDir("var/lib/homenode/catalog", 0700, 0, 0)
	addDir("var/lib/homenode/images", 0710, 0, a.QEMUGID)
	addDir("var/lib/homenode/volumes", 0710, 0, a.QEMUGID)
	if c.Maintenance != nil {
		addDir("var/lib/homenode-backup", 0755, 0, 0)
		addDir("var/lib/homenode-backup/staging", 0700, int(c.Maintenance.UID), c.Maintenance.GID)
	}
	env := fmt.Sprintf("TAILNET_IP=%s\nHTTPS_PORT=%d\nHTTPS_ORIGIN=%s\nPOLICY_GENERATION=%d\nCONTROLLER_UID=%d\nRUNTIME_GID=%d\nTRANSFER_GID=%d\n", c.Network.Bind, c.Network.Port, c.Network.Origin, c.Policy.Generation, a.ControllerUID, a.RuntimeGID, a.TransferGID)
	policy, err := json.MarshalIndent(c.Policy, "", "  ")
	if err != nil {
		return result, err
	}
	addFile("etc/homenode/services.env", 0644, []byte(env))
	addFile("etc/homenode/runtime-policy.json", 0600, append(policy, '\n'))
	addFile("etc/homenode/catalog.pub", 0644, []byte(hex.EncodeToString(c.Publisher)+"\n"))
	addFile("etc/homenode/catalog-floor", 0600, []byte(strconv.FormatInt(c.MinimumCatalogVersion, 10)+"\n"))
	addFile("var/lib/homenode/catalog/catalog.json", 0600, c.Catalog)
	for _, name := range []string{"homenode-control.service", "homenode-supervisor.service", "homenode-transfer.service"} {
		data, err := servicetemplates.Unit(name)
		if err != nil {
			return result, err
		}
		if name == "homenode-supervisor.service" && c.Maintenance != nil {
			data, err = maintenanceUnit(data, *c.Maintenance)
			if err != nil {
				return result, err
			}
		}
		if name == "homenode-control.service" && c.Maintenance != nil {
			data, err = maintenanceControlUnit(data, *c.Maintenance)
			if err != nil {
				return result, err
			}
		}
		addFile("etc/systemd/system/"+name, 0644, data)
	}
	if c.Maintenance != nil {
		addFile("etc/systemd/system/homenode-app-maintenance.socket", 0644, maintenanceSocketUnit())
	}
	slice := fmt.Sprintf("[Unit]\nDescription=HomeNode workload resource boundary\n\n[Slice]\nMemoryMax=%d\nCPUQuota=%d%%\nTasksMax=512\n", int64(c.Policy.MemoryMiB)*(1<<20), c.Policy.VCPUs*100)
	addFile("etc/systemd/system/homenode.slice", 0644, []byte(slice))
	if _, _, err = planRecords(plan, 0); err != nil {
		return result, err
	}
	result = ConfigurationPreview{Maintenance: c.Maintenance, Network: c.Network, RuntimePolicy: c.Policy, Accounts: c.Accounts, ProvidedCapacity: c.Capacity, Plan: plan, CatalogVersion: m.Version, PublisherKeyID: catalog.KeyID(c.Publisher), RequiredDiskBytes: required, VerifiedImageBytes: imageCredit, RequiredFreeDiskBytes: required - imageCredit, Pending: []string{"verify actual service account memberships", "verify supported host enforcement and measured VM overhead", "place and verify immutable guest images", "verify live Tailscale and protected HTTPS identity", "verify restrictive tailnet policy from allowed and denied devices", "validate and activate services", "complete passkey enrollment and phone sample job"}}
	return result, nil
}
