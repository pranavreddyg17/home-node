package install

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"runtime"
	"syscall"
)

const configurationRemovalJournalLimit = 64 << 10

type configurationRemovalIntent struct {
	Version   int                      `json:"version"`
	Completed int                      `json:"completed,omitempty"`
	Ready     bool                     `json:"ready,omitempty"`
	Plan      ConfigurationRemovalPlan `json:"plan"`
}

// PrepareConfigurationRemoval commits the exact verified configuration plan.
// It does not stop services, unlink configuration or erase any data.
func (e *Engine) PrepareConfigurationRemoval(ctx context.Context) (ConfigurationRemovalPlan, error) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 || e.host.Name() != "/" {
		return ConfigurationRemovalPlan{}, ErrAccounts
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.prepareConfigurationRemovalLocked(ctx)
}
func (e *Engine) prepareConfigurationRemovalLocked(ctx context.Context) (ConfigurationRemovalPlan, error) {
	var empty ConfigurationRemovalPlan
	plan, err := e.planConfigurationRemovalLocked(ctx)
	if err != nil {
		return empty, err
	}
	expected, err := json.Marshal(configurationRemovalIntent{Version: 1, Plan: plan})
	if err != nil {
		return empty, err
	}
	data, err := e.readConfigurationRemovalIntent()
	if err == nil {
		if !bytes.Equal(data, expected) {
			return empty, ErrConflict
		}
		return plan, nil
	}
	if !os.IsNotExist(err) {
		return empty, err
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if err := e.saveJournalBytes("configuration-removal", expected); err != nil {
		return empty, err
	}
	return plan, nil
}
func (e *Engine) readConfigurationRemovalIntent() ([]byte, error) {
	file, err := e.journalRoot.OpenFile("configuration-removal.json", os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !accountJournalFileAdmitted(info, e.owner, configurationRemovalJournalLimit) {
		return nil, ErrConflict
	}
	data, err := io.ReadAll(io.LimitReader(file, configurationRemovalJournalLimit+1))
	if err != nil || len(data) > configurationRemovalJournalLimit || !e.accountJournalPathUnchanged("configuration-removal.json", info, configurationRemovalJournalLimit) {
		return nil, ErrConflict
	}
	return data, nil
}
func (e *Engine) loadConfigurationRemovalIntent(installed journal) (configurationRemovalIntent, error) {
	var intent configurationRemovalIntent
	plan, err := configurationRemovalPlanFor(installed)
	if err != nil {
		return intent, err
	}
	data, err := e.readConfigurationRemovalIntent()
	if err != nil {
		return intent, err
	}
	if json.Unmarshal(data, &intent) != nil || intent.Version != 1 || intent.Completed < 0 || intent.Completed > len(plan.Files) || (intent.Ready && intent.Completed != len(plan.Files)) {
		return intent, ErrConflict
	}
	expected := intent
	expected.Plan = plan
	canonical, err := json.Marshal(expected)
	if err != nil || !bytes.Equal(data, canonical) {
		return intent, ErrConflict
	}
	return expected, nil
}
