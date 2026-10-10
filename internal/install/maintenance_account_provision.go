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
	"time"
)

type maintenanceProvisionBackend interface {
	Snapshot(context.Context) (accountSnapshot, error)
	Lookup(context.Context, string, string) (bool, error)
	Execute(context.Context, AccountCommand) error
	VerifyMaintenance(context.Context) (MaintenanceAccount, error)
}
type nativeMaintenanceProvisioner struct{ nativeAccountProvisioner }

func (nativeMaintenanceProvisioner) VerifyMaintenance(ctx context.Context) (MaintenanceAccount, error) {
	return InspectMaintenanceAccount(ctx)
}

func (e *Engine) loadMaintenanceAccountJournal(base accountJournal) (maintenanceAccountJournal, error) {
	var j maintenanceAccountJournal
	file, err := e.journalRoot.OpenFile("maintenance-accounts.json", os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return j, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !accountJournalFileAdmitted(info, e.owner, 16384) {
		return j, ErrConflict
	}
	data, err := io.ReadAll(io.LimitReader(file, 16385))
	if err != nil || len(data) > 16384 || json.Unmarshal(data, &j) != nil {
		return j, ErrConflict
	}
	if !e.accountJournalPathUnchanged("maintenance-accounts.json", info, 16384) {
		return j, ErrConflict
	}
	p := j.Plan
	if j.Version != 1 || p.OwnerID != base.OwnerID || j.Completed < 0 || j.Completed > 2 || (j.Ready && j.Completed != 2) || p.Identity.UID < 100 || p.Identity.UID >= 1000 || p.Identity.GID < 100 || p.Identity.GID >= 1000 || p.Identity.UID == base.Accounts.ControllerUID || p.Identity.UID == base.Accounts.TransferUID {
		return j, ErrConflict
	}
	for _, gid := range []int{base.Accounts.ControllerGID, base.Accounts.TransferGID, base.Accounts.RuntimeGID, base.Accounts.QEMUGID} {
		if p.Identity.GID == gid {
			return j, ErrConflict
		}
	}
	expected := j
	expected.Plan.Commands = maintenanceCreationCommands(p.OwnerID, p.Identity)
	canonical, err := json.Marshal(expected)
	if err != nil || !bytes.Equal(data, canonical) {
		return j, ErrConflict
	}
	return expected, nil
}

// ProvisionMaintenanceAccount runs only a previously committed creation intent.
// Failures retain that intent; an uncertain command result is reconciled against
// the exact owned account attributes before any command is retried.
func (e *Engine) ProvisionMaintenanceAccount(ctx context.Context) (MaintenanceAccount, error) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 || e.host.Name() != "/" {
		return MaintenanceAccount{}, ErrAccounts
	}
	deadline, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	return e.provisionMaintenanceAccount(deadline, nativeMaintenanceProvisioner{})
}
func (e *Engine) provisionMaintenanceAccount(ctx context.Context, b maintenanceProvisionBackend) (MaintenanceAccount, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	var empty MaintenanceAccount
	base, err := e.loadAccountJournal()
	if err != nil || !base.Ready {
		return empty, ErrConflict
	}
	j, err := e.loadMaintenanceAccountJournal(base)
	if err != nil {
		return empty, err
	}
	for index, command := range j.Plan.Commands {
		if err = ctx.Err(); err != nil {
			return empty, err
		}
		snapshot, err := b.Snapshot(ctx)
		defer clear(snapshot.shadow)
		if err != nil {
			return empty, err
		}
		for step := 0; step < 5; step++ {
			ready, err := e.ownedAccountStepMatches(snapshot, base, step)
			if err != nil || !ready {
				return empty, ErrConflict
			}
		}
		present, err := maintenanceAccountStepMatches(snapshot, j.Plan, index)
		if err != nil {
			return empty, err
		}
		if !present {
			if index < j.Completed || j.Ready {
				return empty, ErrConflict
			}
			database, id := "group", strconv.Itoa(j.Plan.Identity.GID)
			if index == 1 {
				database = "passwd"
				id = strconv.FormatUint(uint64(j.Plan.Identity.UID), 10)
			}
			for _, key := range []string{"homenode-backup", id} {
				exists, err := b.Lookup(ctx, database, key)
				if err != nil {
					return empty, err
				}
				if exists {
					return empty, ErrConflict
				}
			}
			if err = ctx.Err(); err != nil {
				return empty, err
			}
			if err = b.Execute(ctx, command); err != nil {
				return empty, err
			}
			snapshot, err = b.Snapshot(ctx)
			defer clear(snapshot.shadow)
			if err != nil {
				return empty, err
			}
			present, err = maintenanceAccountStepMatches(snapshot, j.Plan, index)
			if err != nil || !present {
				return empty, ErrConflict
			}
			if e.checkpoint != nil {
				if err = e.checkpoint("maintenance-account-created", "homenode-backup"); err != nil {
					return empty, err
				}
			}
		}
		if index >= j.Completed {
			j.Completed = index + 1
			data, err := json.Marshal(j)
			if err != nil {
				return empty, err
			}
			if err = e.saveJournalBytes("maintenance-accounts", data); err != nil {
				return empty, err
			}
		}
	}
	actual, err := b.VerifyMaintenance(ctx)
	if err != nil {
		return empty, err
	}
	if actual != j.Plan.Identity {
		return empty, ErrConflict
	}
	if err = ctx.Err(); err != nil {
		return empty, err
	}
	if !j.Ready {
		j.Ready = true
		data, err := json.Marshal(j)
		if err != nil {
			return empty, err
		}
		if err = e.saveJournalBytes("maintenance-accounts", data); err != nil {
			return empty, err
		}
	}
	return actual, nil
}
