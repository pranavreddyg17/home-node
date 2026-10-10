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

type gatewayAccountJournal struct {
	Version   int                `json:"version"`
	Completed int                `json:"completed,omitempty"`
	Ready     bool               `json:"ready,omitempty"`
	Plan      GatewayAccountPlan `json:"plan"`
}

// PrepareGatewayAccount commits creation intent without executing commands.
// It requires the already-owned controller/transfer account phase to be ready.
func (e *Engine) PrepareGatewayAccount(ctx context.Context) (GatewayAccountPlan, error) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 || e.host.Name() != "/" {
		return GatewayAccountPlan{}, ErrAccounts
	}
	return e.prepareGatewayAccount(ctx, nativeAccountProvisioner{})
}
func (e *Engine) prepareGatewayAccount(ctx context.Context, b accountProvisionBackend) (GatewayAccountPlan, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	var empty GatewayAccountPlan
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
		matched, matchErr := accountStepMatches(snapshot, base, index)
		if matchErr != nil || !matched {
			return empty, ErrConflict
		}
	}
	current, err := ValidateLocalAccounts(snapshot.passwd, snapshot.groups, snapshot.shadow)
	if err != nil || current != base.Accounts {
		return empty, ErrConflict
	}
	plan, err := PlanGatewayAccountCreation(base.OwnerID, snapshot.passwd, snapshot.groups, snapshot.shadow, snapshot.nss)
	if err != nil {
		return empty, err
	}
	// The journal cannot substitute general commands, another install owner, or
	// different IDs. Canonical bytes also reject duplicate/unknown JSON fields.
	expected, err := json.Marshal(gatewayAccountJournal{Version: 1, Plan: plan})
	if err != nil {
		return empty, err
	}
	file, err := e.journalRoot.OpenFile("gateway-accounts.json", os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	existing := false
	if err == nil {
		info, statErr := file.Stat()
		if statErr != nil || !accountJournalFileAdmitted(info, e.owner, 16384) {
			file.Close()
			return empty, ErrConflict
		}
		data, readErr := io.ReadAll(io.LimitReader(file, 16385))
		closeErr := file.Close()
		if readErr != nil || closeErr != nil || len(data) > 16384 || !bytes.Equal(data, expected) || !e.accountJournalPathUnchanged("gateway-accounts.json", info, 16384) {
			return empty, ErrConflict
		}
		existing = true
	} else if !os.IsNotExist(err) {
		return empty, err
	}
	for _, lookup := range [][2]string{{"passwd", "homenode-gateway"}, {"passwd", strconv.FormatUint(uint64(plan.Identity.UID), 10)}, {"group", "homenode-gateway"}, {"group", strconv.Itoa(plan.Identity.GID)}, {"group", "homenode-proxy"}, {"group", strconv.Itoa(plan.Identity.ProxyGID)}} {
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
		if err = e.saveJournalBytes("gateway-accounts", expected); err != nil {
			return empty, err
		}
	}
	return plan, nil
}

func (e *Engine) loadGatewayAccountJournal(base accountJournal) (gatewayAccountJournal, error) {
	var j gatewayAccountJournal
	file, err := e.journalRoot.OpenFile("gateway-accounts.json", os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return j, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !accountJournalFileAdmitted(info, e.owner, 16384) {
		return j, ErrConflict
	}
	data, err := io.ReadAll(io.LimitReader(file, 16385))
	if err != nil || len(data) > 16384 || json.Unmarshal(data, &j) != nil || !e.accountJournalPathUnchanged("gateway-accounts.json", info, 16384) {
		return j, ErrConflict
	}
	p := j.Plan
	if j.Version != 1 || p.OwnerID != base.OwnerID || j.Completed < 0 || j.Completed > 4 || (j.Ready && j.Completed != 4) || p.Identity.UID < 100 || p.Identity.UID > 999 || p.Identity.GID < 100 || p.Identity.GID > 999 || p.Identity.ProxyGID < 100 || p.Identity.ProxyGID > 999 || p.Identity.GID == p.Identity.ProxyGID || p.Identity.UID == base.Accounts.ControllerUID || p.Identity.UID == base.Accounts.TransferUID {
		return j, ErrConflict
	}
	for _, gid := range []int{base.Accounts.ControllerGID, base.Accounts.TransferGID, base.Accounts.RuntimeGID, base.Accounts.QEMUGID} {
		if p.Identity.GID == gid || p.Identity.ProxyGID == gid {
			return j, ErrConflict
		}
	}
	expected := j
	expected.Plan.Commands = gatewayCreationCommands(p.OwnerID, p.Identity)
	canonical, err := json.Marshal(expected)
	if err != nil || !bytes.Equal(data, canonical) {
		return j, ErrConflict
	}
	return expected, nil
}
