package install

import (
	"encoding/hex"
	"strconv"
	"strings"
)

type MaintenanceAccountPlan struct {
	OwnerID  string             `json:"ownerId"`
	Identity MaintenanceAccount `json:"identity"`
	Commands []AccountCommand   `json:"commands"`
}

// PlanMaintenanceAccountCreation performs no mutation. A caller must persist
// ownership intent, confirm live NSS vacancy, execute with deadlines, and verify
// the resulting account before configuring maintenance access. Existing names
// are never adopted, including dangling group members or shadow entries.
func PlanMaintenanceAccountCreation(owner string, passwd, groups, shadow, nss []byte) (MaintenanceAccountPlan, error) {
	var empty MaintenanceAccountPlan
	if len(owner) != 32 {
		return empty, ErrPlan
	}
	if _, err := hex.DecodeString(owner); err != nil {
		return empty, ErrPlan
	}
	if err := ValidateNameServices(nss); err != nil {
		return empty, err
	}
	if _, err := ValidateLocalAccounts(passwd, groups, shadow); err != nil {
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
	secrets, err := accountLines(shadow, 9)
	if err != nil {
		return empty, err
	}
	usedUID, usedGID := map[int]bool{}, map[int]bool{}
	for _, row := range users {
		if row[0] == "homenode-backup" {
			return empty, ErrConflict
		}
		uid, e1 := accountID(row[2])
		gid, e2 := accountID(row[3])
		if e1 != nil || e2 != nil {
			return empty, ErrAccounts
		}
		usedUID[uid] = true
		usedGID[gid] = true
	}
	for _, row := range groupRows {
		if row[0] == "homenode-backup" {
			return empty, ErrConflict
		}
		gid, err := accountID(row[2])
		if err != nil {
			return empty, err
		}
		usedGID[gid] = true
		for _, member := range strings.Split(row[3], ",") {
			if member == "homenode-backup" {
				return empty, ErrConflict
			}
		}
	}
	for _, row := range secrets {
		if row[0] == "homenode-backup" {
			return empty, ErrConflict
		}
	}
	reserve := func(used map[int]bool) int {
		for id := 999; id >= 100; id-- {
			if !used[id] {
				return id
			}
		}
		return 0
	}
	uid, gid := reserve(usedUID), reserve(usedGID)
	if uid == 0 || gid == 0 {
		return empty, ErrAccounts
	}
	identity := MaintenanceAccount{UID: uint32(uid), GID: gid}
	return MaintenanceAccountPlan{OwnerID: owner, Identity: identity, Commands: maintenanceCreationCommands(owner, identity)}, nil
}

func maintenanceCreationCommands(owner string, identity MaintenanceAccount) []AccountCommand {
	return []AccountCommand{
		{Program: "/usr/sbin/groupadd", Arguments: []string{"--system", "--gid", strconv.Itoa(identity.GID), "homenode-backup"}},
		{Program: "/usr/sbin/useradd", Arguments: []string{"--system", "--uid", strconv.FormatUint(uint64(identity.UID), 10), "--gid", "homenode-backup", "--no-create-home", "--home-dir", "/nonexistent", "--shell", "/usr/sbin/nologin", "--comment", "HomeNode install " + owner, "homenode-backup"}},
	}
}
