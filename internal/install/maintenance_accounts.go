package install

import "strings"

// MaintenanceAccount is the isolated local backup identity. It has only its
// private primary group; runtime/controller/transfer groups are forbidden.
type MaintenanceAccount struct {
	UID uint32 `json:"uid"`
	GID int    `json:"gid"`
}

// ValidateMaintenanceAccount checks the backup identity together with the
// existing controller/transfer isolation policy. Provisioning must still verify
// NSS agreement and journal ownership before adopting or creating this account.
func ValidateMaintenanceAccount(passwd, group, shadow []byte) (MaintenanceAccount, error) {
	var empty MaintenanceAccount
	if _, err := ValidateLocalAccounts(passwd, group, shadow); err != nil {
		return empty, err
	}
	users, err := accountLines(passwd, 7)
	if err != nil {
		return empty, err
	}
	groups, err := accountLines(group, 4)
	if err != nil {
		return empty, err
	}
	secrets, err := accountLines(shadow, 9)
	if err != nil {
		return empty, err
	}
	var backup []string
	for _, row := range users {
		if row[0] == "homenode-backup" {
			backup = row
		}
	}
	if backup == nil || backup[1] != "x" || backup[5] != "/nonexistent" || (backup[6] != "/usr/sbin/nologin" && backup[6] != "/sbin/nologin") {
		return empty, ErrAccounts
	}
	uid, err := accountID(backup[2])
	if err != nil || uid < 1 || uid >= 1000 {
		return empty, ErrAccounts
	}
	gid, err := accountID(backup[3])
	if err != nil || gid < 1 || gid >= 1000 {
		return empty, ErrAccounts
	}
	for _, row := range users {
		otherUID, e1 := accountID(row[2])
		otherGID, e2 := accountID(row[3])
		if e1 != nil || e2 != nil || (row[0] != "homenode-backup" && (otherUID == uid || otherGID == gid)) {
			return empty, ErrAccounts
		}
	}
	privateGroup := false
	for _, row := range groups {
		otherGID, err := accountID(row[2])
		if err != nil {
			return empty, err
		}
		if row[0] == "homenode-backup" {
			if row[3] != "" && row[3] != "homenode-backup" {
				return empty, ErrAccounts
			}
			if otherGID != gid {
				return empty, ErrAccounts
			}
			privateGroup = true
		} else if otherGID == gid {
			return empty, ErrAccounts
		}
		if row[3] != "" {
			members := strings.Split(row[3], ",")
			for _, member := range members {
				if (row[0] == "homenode-backup" && member != "homenode-backup") || (member == "homenode-backup" && row[0] != "homenode-backup") {
					return empty, ErrAccounts
				}
			}
		}
	}
	if !privateGroup {
		return empty, ErrAccounts
	}
	locked := false
	for _, row := range secrets {
		if row[0] == "homenode-backup" {
			locked = len(row[1]) > 0 && (row[1][0] == '!' || row[1][0] == '*')
		}
	}
	if !locked {
		return empty, ErrAccounts
	}
	return MaintenanceAccount{UID: uint32(uid), GID: gid}, nil
}
