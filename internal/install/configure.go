package install

import (
	"context"
	"os"
	"runtime"
	"time"

	"github.com/pranavreddyg17/home-node/internal/hostcheck"
)

// Configure installs owned configuration only. Publisher and catalog floor
// must come from independent release trust. Caller-supplied account IDs and
// capacity are ignored; no images are installed and no services are activated.
func (e *Engine) Configure(ctx context.Context, c Configuration) (ConfigurationPreview, error) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 || e.host.Name() != "/" {
		return ConfigurationPreview{}, ErrConflict
	}
	return e.configure(ctx, c, time.Now(), func(ctx context.Context) (Accounts, Capacity, error) {
		j, err := e.loadAccountJournal()
		if err != nil {
			return Accounts{}, Capacity{}, err
		}
		snapshot, err := (nativeAccountProvisioner{}).Snapshot(ctx)
		defer clear(snapshot.shadow)
		if err != nil {
			return Accounts{}, Capacity{}, err
		}
		for index := range creationCommands(j.OwnerID, j.Accounts) {
			present, err := accountStepMatches(snapshot, j, index)
			if err != nil || !present {
				return Accounts{}, Capacity{}, ErrAccounts
			}
		}
		a, err := InspectLocalAccounts(ctx)
		if err != nil {
			return Accounts{}, Capacity{}, err
		}
		report := hostcheck.Inspect("/var/lib")
		if !report.PreparationPrerequisitesMet() {
			return Accounts{}, Capacity{}, ErrConflict
		}
		return a, Capacity{MemoryBytes: report.Host.MemoryBytes, FreeDiskBytes: report.Host.AvailableDiskBytes, LogicalCPUs: runtime.NumCPU()}, nil
	})
}

func (e *Engine) configure(ctx context.Context, c Configuration, now time.Time, observe func(context.Context) (Accounts, Capacity, error)) (ConfigurationPreview, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return ConfigurationPreview{}, err
	}
	j, err := e.loadAccountJournal()
	if err != nil {
		return ConfigurationPreview{}, err
	}
	if !j.Ready {
		return ConfigurationPreview{}, ErrAccounts
	}
	a, capacity, err := observe(ctx)
	if err != nil {
		return ConfigurationPreview{}, err
	}
	if a != j.Accounts {
		return ConfigurationPreview{}, ErrAccounts
	}
	c.Accounts = a
	c.Policy.ControllerUID = a.ControllerUID
	c.Policy.TransferUID = a.TransferUID
	c.Capacity = capacity
	credit, cleaned, err := e.configurationImageCredit(ctx, c, now)
	if err != nil {
		return ConfigurationPreview{}, err
	}
	if cleaned {
		a, capacity, err = observe(ctx)
		if err != nil {
			return ConfigurationPreview{}, err
		}
		if a != j.Accounts {
			return ConfigurationPreview{}, ErrAccounts
		}
		c.Capacity = capacity
	}
	preview, err := configurationPlan(c, now, credit)
	if err != nil {
		return ConfigurationPreview{}, err
	}
	if err = e.applyLocked(ctx, preview.Plan); err != nil {
		return ConfigurationPreview{}, err
	}
	preview.Pending = preview.Pending[1:] // Actual identity admission completed above.
	return preview, nil
}
