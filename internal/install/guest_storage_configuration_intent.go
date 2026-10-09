package install

import (
	"context"
	"encoding/json"
	"strings"
)

type guestStorageConfigurationIntent struct {
	Version             int                          `json:"version"`
	StorageIntentSHA256 string                       `json:"storageIntentSha256"`
	Plan                GuestStorageProvisioningPlan `json:"plan"`
	Original            journal                      `json:"original"`
	Desired             journal                      `json:"desired"`
	SourcePolicy        []byte                       `json:"sourcePolicy"`
	SourceEnvironment   []byte                       `json:"sourceEnvironment"`
	Policy              []byte                       `json:"policy"`
	Environment         []byte                       `json:"environment"`
}

// Caller retains proposal, installed bytes, storage and runtime authority.
// Record both configurations and journal states before either publication.
func (e *Engine) commitGuestStorageConfigurationIntent(ctx context.Context, installed journal, sourcePolicy, sourceEnv []byte, plan GuestStorageProvisioningPlan, guard func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if guard == nil || len(installed.ID) != 32 || strings.Trim(installed.ID, "0123456789abcdef") != "" || len(installed.Items) == 0 || len(installed.Items) > maxInstallationItems {
		return ErrPlan
	}
	if err := guard(ctx); err != nil {
		return err
	}
	desired, policy, environment, err := planGuestStorageConfiguration(ctx, installed, sourcePolicy, sourceEnv, plan)
	if err != nil {
		return err
	}
	proposal, err := json.Marshal(guestStorageIntent{Version: 1, Plan: plan})
	if err != nil {
		return err
	}
	data, err := json.Marshal(guestStorageConfigurationIntent{Version: 1, StorageIntentSHA256: digest(proposal), Plan: plan, Original: installed, Desired: desired, SourcePolicy: sourcePolicy, SourceEnvironment: sourceEnv, Policy: policy, Environment: environment})
	if err != nil {
		return err
	}
	if err := guard(ctx); err != nil {
		return err
	}
	if err := e.commitImmutableGuestIntent(ctx, "guest-storage-configuration-intent.json", "guest-storage-configuration", data); err != nil {
		return err
	}
	return guard(ctx)
}
