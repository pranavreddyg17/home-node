package install

import (
	"encoding/hex"
	"strings"
)

type AccountCommand struct {
	Program   string   `json:"program"`
	Arguments []string `json:"arguments"`
}
type AccountCreationPlan struct {
	OwnerID  string           `json:"ownerId"`
	Accounts Accounts         `json:"accounts"`
	Commands []AccountCommand `json:"commands"`
	Pending  []string         `json:"pending"`
}

// PlanAccountCreation makes no changes. The production caller must confirm
// system lookups agree and journal this intent before running any command.
// Occupied names are never adopted, even when their attributes look suitable.
func PlanAccountCreation(ownerID string, passwd, groups, nss []byte) (AccountCreationPlan, error) {
	var empty AccountCreationPlan
	if len(ownerID) != 32 {
		return empty, ErrPlan
	}
	if _, err := hex.DecodeString(ownerID); err != nil {
		return empty, ErrPlan
	}
	if err := ValidateNameServices(nss); err != nil {
		return empty, err
	}
	users, err := accountLines(passwd, 7)
	if err != nil {
		return empty, err
	}
	groupRows, err := accountLines(groups, 4)
	if err != nil {
		return empty, err
	}
	occupiedUID, occupiedGID := map[int]bool{}, map[int]bool{}
	userNames, groupNames := map[string]bool{}, map[string]bool{}
	qemuGID := 0
	for _, row := range users {
		if userNames[row[0]] || row[0] == "homenode" || row[0] == "homenode-transfer" {
			return empty, ErrAccounts
		}
		userNames[row[0]] = true
		uid, err := accountID(row[2])
		if err != nil {
			return empty, err
		}
		gid, err := accountID(row[3])
		if err != nil {
			return empty, err
		}
		occupiedUID[uid] = true
		// A dangling primary GID must not become a private service group.
		occupiedGID[gid] = true
	}
	for _, row := range groupRows {
		if groupNames[row[0]] || row[0] == "homenode" || row[0] == "homenode-transfer" || row[0] == "homenode-runtime" {
			return empty, ErrAccounts
		}
		groupNames[row[0]] = true
		gid, err := accountID(row[2])
		if err != nil {
			return empty, err
		}
		occupiedGID[gid] = true
		if row[0] == "libvirt-qemu" {
			qemuGID = gid
		}
		for _, member := range strings.Split(row[3], ",") {
			if member == "homenode" || member == "homenode-transfer" {
				return empty, ErrAccounts
			}
		}
	}
	if qemuGID == 0 {
		return empty, ErrAccounts
	}
	for _, row := range groupRows {
		gid, _ := accountID(row[2])
		if gid == qemuGID && row[0] != "libvirt-qemu" {
			return empty, ErrAccounts
		}
	}
	reserve := func(used map[int]bool) (int, error) {
		for id := 999; id >= 100; id-- {
			if !used[id] {
				used[id] = true
				return id, nil
			}
		}
		return 0, ErrAccounts
	}
	controllerGID, err := reserve(occupiedGID)
	if err != nil {
		return empty, err
	}
	transferGID, err := reserve(occupiedGID)
	if err != nil {
		return empty, err
	}
	runtimeGID, err := reserve(occupiedGID)
	if err != nil {
		return empty, err
	}
	controllerUID, err := reserve(occupiedUID)
	if err != nil {
		return empty, err
	}
	transferUID, err := reserve(occupiedUID)
	if err != nil {
		return empty, err
	}
	a := Accounts{ControllerUID: uint32(controllerUID), TransferUID: uint32(transferUID), ControllerGID: controllerGID, TransferGID: transferGID, RuntimeGID: runtimeGID, QEMUGID: qemuGID}
	commands := creationCommands(ownerID, a)
	return AccountCreationPlan{OwnerID: ownerID, Accounts: a, Commands: commands, Pending: []string{"confirm live system name and numeric ID vacancy", "commit ownership intent to private journal", "execute fixed commands with deadlines and verify each result", "verify locked credentials and exact memberships", "retain ownership record for safe rollback"}}, nil
}
