package install

import "strings"

// maintenanceAccountStepMatches reconciles a lost command acknowledgement.
// Only the planned private group and owner-marked user can satisfy a step;
// colliding identities or changed isolation are actionable conflicts.
func maintenanceAccountStepMatches(s accountSnapshot, p MaintenanceAccountPlan, step int) (bool, error) {
	if step < 0 || step > 1 {
		return false, ErrPlan
	}
	if _, err := ValidateLocalAccounts(s.passwd, s.groups, s.shadow); err != nil {
		return false, err
	}
	if err := ValidateNameServices(s.nss); err != nil {
		return false, err
	}
	users, err := accountLines(s.passwd, 7)
	if err != nil {
		return false, err
	}
	groups, err := accountLines(s.groups, 4)
	if err != nil {
		return false, err
	}
	secrets, err := accountLines(s.shadow, 9)
	if err != nil {
		return false, err
	}
	foundUser, foundGroup, locked := false, false, false
	for _, row := range users {
		uid, e1 := accountID(row[2])
		gid, e2 := accountID(row[3])
		if e1 != nil || e2 != nil {
			return false, ErrAccounts
		}
		if row[0] != "homenode-backup" {
			if uint32(uid) == p.Identity.UID || gid == p.Identity.GID {
				return false, ErrConflict
			}
			continue
		}
		if uint32(uid) != p.Identity.UID || gid != p.Identity.GID || row[1] != "x" || row[4] != "HomeNode install "+p.OwnerID || row[5] != "/nonexistent" || row[6] != "/usr/sbin/nologin" {
			return false, ErrConflict
		}
		foundUser = true
	}
	for _, row := range groups {
		gid, err := accountID(row[2])
		if err != nil {
			return false, err
		}
		if row[0] == "homenode-backup" {
			if gid != p.Identity.GID || (row[3] != "" && row[3] != "homenode-backup") {
				return false, ErrConflict
			}
			foundGroup = true
		} else {
			if gid == p.Identity.GID {
				return false, ErrConflict
			}
			for _, member := range strings.Split(row[3], ",") {
				if member == "homenode-backup" {
					return false, ErrConflict
				}
			}
		}
	}
	for _, row := range secrets {
		if row[0] == "homenode-backup" {
			if !foundUser {
				return false, ErrConflict
			}
			locked = strings.HasPrefix(row[1], "!") || strings.HasPrefix(row[1], "*")
		}
	}
	if foundUser && (!foundGroup || !locked) {
		return false, ErrConflict
	}
	if step == 0 {
		return foundGroup, nil
	}
	return foundUser, nil
}
