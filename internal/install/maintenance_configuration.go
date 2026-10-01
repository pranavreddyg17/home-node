package install

import (
	"bytes"
	"context"
	"fmt"
	"os"
)

func maintenanceUnit(data []byte, identity MaintenanceAccount) ([]byte, error) {
	line := []byte("ExecStart=/usr/lib/homenode/homenode-supervisor --access-gid ${RUNTIME_GID} --transfer-gid ${TRANSFER_GID}\n")
	if bytes.Count(data, line) != 1 {
		return nil, ErrPlan
	}
	replacement := []byte(fmt.Sprintf("ExecStart=/usr/lib/homenode/homenode-supervisor --access-gid ${RUNTIME_GID} --transfer-gid ${TRANSFER_GID} --maintenance-uid %d --maintenance-gid %d\n", identity.UID, identity.GID))
	return bytes.Replace(data, line, replacement, 1), nil
}
func (e *Engine) observeMaintenanceAccount(ctx context.Context, base accountJournal) (*MaintenanceAccount, error) {
	j, err := e.loadMaintenanceAccountJournal(base)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !j.Ready {
		return nil, ErrAccounts
	}
	snapshot, err := (nativeAccountProvisioner{}).Snapshot(ctx)
	defer clear(snapshot.shadow)
	if err != nil {
		return nil, err
	}
	for step := 0; step < 2; step++ {
		ready, err := maintenanceAccountStepMatches(snapshot, j.Plan, step)
		if err != nil || !ready {
			return nil, ErrAccounts
		}
	}
	identity, err := InspectMaintenanceAccount(ctx)
	if err != nil {
		return nil, err
	}
	if identity != j.Plan.Identity {
		return nil, ErrAccounts
	}
	return &identity, nil
}

func maintenanceControlUnit(data []byte, identity MaintenanceAccount) ([]byte, error) {
	line := []byte(" --policy-generation ${POLICY_GENERATION}\n")
	requires := []byte("Requires=homenode-supervisor.service homenode-transfer.service\n")
	after := []byte("After=network-online.target tailscaled.service homenode-supervisor.service homenode-transfer.service\n")
	if bytes.Count(data, line) != 1 || bytes.Count(data, requires) != 1 || bytes.Count(data, after) != 1 || bytes.Count(data, []byte("Type=simple\n")) != 1 {
		return nil, ErrPlan
	}
	data = bytes.Replace(data, line, []byte(fmt.Sprintf(" --policy-generation ${POLICY_GENERATION} --maintenance-uid %d --maintenance-gid %d\n", identity.UID, identity.GID)), 1)
	data = bytes.Replace(data, after, []byte("After=network-online.target tailscaled.service homenode-supervisor.service homenode-transfer.service homenode-app-maintenance.socket\n"), 1)
	data = bytes.Replace(data, []byte("Type=simple\n"), []byte("Type=simple\nSockets=homenode-app-maintenance.socket\n"), 1)
	return bytes.Replace(data, requires, []byte("Requires=homenode-supervisor.service homenode-transfer.service homenode-app-maintenance.socket\n"), 1), nil
}
func maintenanceSocketUnit() []byte {
	return []byte("[Unit]\nDescription=HomeNode backup-only app maintenance listener\n\n[Socket]\nListenStream=/run/homenode-backup/apps.sock\nFileDescriptorName=homenode-app-maintenance\nSocketUser=root\nSocketGroup=homenode-backup\nSocketMode=0660\nDirectoryMode=0755\nService=homenode-control.service\nRemoveOnStop=yes\n\n[Install]\nWantedBy=sockets.target\n")
}
