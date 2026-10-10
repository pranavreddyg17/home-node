package install

import (
	"encoding/hex"
	"strings"
)

// gatewayAccountStepMatches reconciles lost acknowledgements without accepting
// an unowned user or granting the gateway access to another service group.
// Partial snapshots are checked here only; operational validation remains strict.
func gatewayAccountStepMatches(s accountSnapshot, p GatewayAccountPlan, step int) (bool, error) {
	if step < 0 || step > 3 || len(p.OwnerID) != 32 || p.Identity.UID < 100 || p.Identity.UID > 999 || p.Identity.GID < 100 || p.Identity.GID > 999 || p.Identity.ProxyGID < 100 || p.Identity.ProxyGID > 999 || p.Identity.GID == p.Identity.ProxyGID {
		return false, ErrPlan
	}
	if _, err := hex.DecodeString(p.OwnerID); err != nil {
		return false, ErrPlan
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
	var baseUsers, baseGroups, baseSecrets strings.Builder
	foundUser, foundGroup, foundProxy, controllerMember, gatewayMember, locked := false, false, false, false, false, false
	privateMember := false
	names := map[string]bool{}
	for _, row := range users {
		if names[row[0]] {
			return false, ErrConflict
		}
		names[row[0]] = true
		uid, e1 := accountID(row[2])
		gid, e2 := accountID(row[3])
		if e1 != nil || e2 != nil {
			return false, ErrAccounts
		}
		if row[0] == "homenode-gateway" {
			if uint32(uid) != p.Identity.UID || gid != p.Identity.GID || row[1] != "x" || row[4] != "HomeNode install "+p.OwnerID || row[5] != "/nonexistent" || row[6] != "/usr/sbin/nologin" {
				return false, ErrConflict
			}
			foundUser = true
		} else {
			if uint32(uid) == p.Identity.UID || gid == p.Identity.GID || gid == p.Identity.ProxyGID || row[0] == "homenode-proxy" {
				return false, ErrConflict
			}
			baseUsers.WriteString(strings.Join(row, ":"))
			baseUsers.WriteByte('\n')
		}
	}
	names = map[string]bool{}
	for _, row := range groups {
		if names[row[0]] {
			return false, ErrConflict
		}
		names[row[0]] = true
		gid, err := accountID(row[2])
		if err != nil {
			return false, err
		}
		switch row[0] {
		case "homenode-gateway":
			if gid != p.Identity.GID || (row[3] != "" && row[3] != "homenode-gateway") {
				return false, ErrConflict
			}
			foundGroup = true
			privateMember = row[3] != ""
		case "homenode-proxy":
			if gid != p.Identity.ProxyGID {
				return false, ErrConflict
			}
			foundProxy = true
			if row[3] != "" {
				for _, member := range strings.Split(row[3], ",") {
					switch member {
					case "homenode":
						if controllerMember {
							return false, ErrConflict
						}
						controllerMember = true
					case "homenode-gateway":
						if gatewayMember {
							return false, ErrConflict
						}
						gatewayMember = true
					default:
						return false, ErrConflict
					}
				}
			}
		default:
			if gid == p.Identity.GID || gid == p.Identity.ProxyGID {
				return false, ErrConflict
			}
			for _, member := range strings.Split(row[3], ",") {
				if member == "homenode-gateway" || member == "homenode-proxy" {
					return false, ErrConflict
				}
			}
			baseGroups.WriteString(strings.Join(row, ":"))
			baseGroups.WriteByte('\n')
		}
	}
	names = map[string]bool{}
	for _, row := range secrets {
		if names[row[0]] {
			return false, ErrConflict
		}
		names[row[0]] = true
		if row[0] == "homenode-gateway" {
			if !foundUser {
				return false, ErrConflict
			}
			locked = strings.HasPrefix(row[1], "!") || strings.HasPrefix(row[1], "*")
		} else {
			if row[0] == "homenode-proxy" {
				return false, ErrConflict
			}
			baseSecrets.WriteString(strings.Join(row, ":"))
			baseSecrets.WriteByte('\n')
		}
	}
	if _, err := validateBaseLocalAccounts([]byte(baseUsers.String()), []byte(baseGroups.String()), []byte(baseSecrets.String())); err != nil {
		return false, err
	}
	if (foundProxy && !foundGroup) || (foundUser && (!foundGroup || !foundProxy || !locked || !gatewayMember)) || (!foundUser && (gatewayMember || controllerMember || privateMember)) || (controllerMember && !gatewayMember) {
		return false, ErrConflict
	}
	return []bool{foundGroup, foundProxy, foundUser, controllerMember}[step], nil
}

// Proxy membership is validated by gatewayAccountStepMatches. The original
// ownership matcher still verifies the controller and transfer owner markers.
func gatewayBaseStepMatches(s accountSnapshot, base accountJournal, step int) (bool, error) {
	rows, err := accountLines(s.groups, 4)
	if err != nil {
		return false, err
	}
	var groups strings.Builder
	for _, row := range rows {
		if row[0] != "homenode-proxy" {
			groups.WriteString(strings.Join(row, ":"))
			groups.WriteByte('\n')
		}
	}
	s.groups = []byte(groups.String())
	return accountStepMatches(s, base, step)
}
