package install

import (
	"context"
	"os"
	"runtime"
	"strings"
)

type ConfigurationRemovalFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Mode   uint32 `json:"mode"`
	UID    int    `json:"uid"`
	GID    int    `json:"gid"`
}
type ConfigurationRemovalPlan struct {
	InstallationID     string                     `json:"installationId"`
	InstallationDigest string                     `json:"installationDigest"`
	Files              []ConfigurationRemovalFile `json:"files"`
	RetainedPaths      []string                   `json:"retainedPaths"`
}

// PlanConfigurationRemoval verifies installation provenance and lists only
// installer-created configuration files. Every directory and data path remains
// retained, including occupied application and backup directories. This read-only
// plan is not authorization to unlink files without retained service exclusion.
func (e *Engine) PlanConfigurationRemoval(ctx context.Context) (ConfigurationRemovalPlan, error) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 || e.host.Name() != "/" {
		return ConfigurationRemovalPlan{}, ErrAccounts
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.planConfigurationRemovalLocked(ctx)
}
func (e *Engine) planConfigurationRemovalLocked(ctx context.Context) (ConfigurationRemovalPlan, error) {
	var empty ConfigurationRemovalPlan
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	installed, err := e.load()
	if err != nil {
		return empty, err
	}
	if installed.Phase != "installed" {
		return empty, ErrConflict
	}
	plan := ConfigurationRemovalPlan{InstallationID: installed.ID, InstallationDigest: installed.Digest, Files: []ConfigurationRemovalFile{}, RetainedPaths: []string{}}
	for _, item := range installed.Items {
		if err := ctx.Err(); err != nil {
			return empty, err
		}
		if err := e.matches(item); err != nil {
			return empty, err
		}
		// Existing paths are not adopted by installer removal. Neither directories
		// nor catalog/data files under var/lib are deleted by ordinary uninstall.
		if item.State == "existing" || item.Directory || !strings.HasPrefix(item.Path, "etc/") {
			plan.RetainedPaths = append(plan.RetainedPaths, item.Path)
			continue
		}
		if item.State != "created" {
			return empty, ErrConflict
		}
		plan.Files = append(plan.Files, ConfigurationRemovalFile{Path: item.Path, SHA256: item.SHA256, Mode: item.Mode, UID: item.UID, GID: item.GID})
	}
	return plan, nil
}
