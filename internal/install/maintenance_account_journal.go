package install

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"runtime"
	"strconv"
	"syscall"
)

type maintenanceAccountJournal struct {
	Version   int                    `json:"version"`
	Completed int                    `json:"completed,omitempty"`
	Ready     bool                   `json:"ready,omitempty"`
	Plan      MaintenanceAccountPlan `json:"plan"`
}

// PrepareMaintenanceAccount commits creation intent without executing commands.
// It requires the already-owned controller/transfer account phase to be ready.
func (e *Engine) PrepareMaintenanceAccount(ctx context.Context) (MaintenanceAccountPlan, error) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 || e.host.Name() != "/" {
		return MaintenanceAccountPlan{}, ErrAccounts
	}
	return e.prepareMaintenanceAccount(ctx, nativeAccountProvisioner{})
}
func (e *Engine) prepareMaintenanceAccount(ctx context.Context, b accountProvisionBackend) (MaintenanceAccountPlan, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	var empty MaintenanceAccountPlan
	base, err := e.loadAccountJournal()
	if err != nil || !base.Ready {
		return empty, ErrConflict
	}
	snapshot, err := b.Snapshot(ctx)
	defer clear(snapshot.shadow)
	if err != nil {
		return empty, err
	}
	for index := 0; index < 5; index++ {
		matched, matchErr := e.ownedAccountStepMatches(snapshot, base, index)
		if matchErr != nil || !matched {
			return empty, ErrConflict
		}
	}
	current, err := ValidateLocalAccounts(snapshot.passwd, snapshot.groups, snapshot.shadow)
	if err != nil || current != base.Accounts {
		return empty, ErrConflict
	}
	plan, err := PlanMaintenanceAccountCreation(base.OwnerID, snapshot.passwd, snapshot.groups, snapshot.shadow, snapshot.nss)
	if err != nil {
		return empty, err
	}
	// The journal cannot substitute general commands, another install owner, or
	// different IDs. Canonical bytes also reject duplicate/unknown JSON fields.
	expected, err := json.Marshal(maintenanceAccountJournal{Version: 1, Plan: plan})
	if err != nil {
		return empty, err
	}
	file, err := e.journalRoot.OpenFile("maintenance-accounts.json", os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	existing := false
	if err == nil {
		info, statErr := file.Stat()
		if statErr != nil || !accountJournalFileAdmitted(info, e.owner, 16384) {
			file.Close()
			return empty, ErrConflict
		}
		data, readErr := io.ReadAll(io.LimitReader(file, 16385))
		closeErr := file.Close()
		if readErr != nil || closeErr != nil || len(data) > 16384 || !bytes.Equal(data, expected) || !e.accountJournalPathUnchanged("maintenance-accounts.json", info, 16384) {
			return empty, ErrConflict
		}
		existing = true
	} else if !os.IsNotExist(err) {
		return empty, err
	}
	for _, lookup := range [][2]string{{"passwd", "homenode-backup"}, {"passwd", strconv.FormatUint(uint64(plan.Identity.UID), 10)}, {"group", "homenode-backup"}, {"group", strconv.Itoa(plan.Identity.GID)}} {
		occupied, err := b.Lookup(ctx, lookup[0], lookup[1])
		if err != nil {
			return empty, err
		}
		if occupied {
			return empty, ErrConflict
		}
	}
	if err = ctx.Err(); err != nil {
		return empty, err
	}
	if !existing {
		if err = e.saveJournalBytes("maintenance-accounts", expected); err != nil {
			return empty, err
		}
	}
	return plan, nil
}
