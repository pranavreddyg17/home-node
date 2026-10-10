package install

import "strings"

// ValidateLocalAccounts accepts the original identities or the fully isolated
// gateway extension. A partially provisioned extension is never operational.
func ValidateLocalAccounts(passwd, group, shadow []byte) (Accounts, error) {
	rows, err := accountLines(group, 4)
	if err != nil {
		return Accounts{}, err
	}
	users, err := accountLines(passwd, 7)
	if err != nil {
		return Accounts{}, err
	}
	extended := false
	for _, row := range rows {
		if row[0] == "homenode-proxy" || row[0] == "homenode-gateway" {
			extended = true
		}
	}
	for _, row := range users {
		if row[0] == "homenode-gateway" {
			extended = true
		}
	}
	if !extended {
		return validateBaseLocalAccounts(passwd, group, shadow)
	}
	accounts, _, err := validateGatewayAccounts(passwd, group, shadow)
	return accounts, err
}

func ValidateGatewayAccount(passwd, group, shadow []byte) (GatewayAccount, error) {
	_, identity, err := validateGatewayAccounts(passwd, group, shadow)
	return identity, err
}

func validateGatewayAccounts(passwd, group, shadow []byte) (Accounts, GatewayAccount, error) {
	deny := func() (Accounts, GatewayAccount, error) { return Accounts{}, GatewayAccount{}, ErrAccounts }
	users, err := accountLines(passwd, 7)
	if err != nil {
		return deny()
	}
	groups, err := accountLines(group, 4)
	if err != nil {
		return deny()
	}
	secrets, err := accountLines(shadow, 9)
	if err != nil {
		return deny()
	}
	var gateway []string
	for _, row := range users {
		if row[0] == "homenode-gateway" {
			if gateway != nil {
				return deny()
			}
			gateway = row
		}
	}
	if gateway == nil || gateway[1] != "x" || gateway[5] != "/nonexistent" || (gateway[6] != "/usr/sbin/nologin" && gateway[6] != "/sbin/nologin") {
		return deny()
	}
	uid, e1 := accountID(gateway[2])
	gid, e2 := accountID(gateway[3])
	if e1 != nil || e2 != nil || uid < 100 || uid > 999 || gid < 100 || gid > 999 {
		return deny()
	}
	private, proxy := false, 0
	var base strings.Builder
	for _, row := range groups {
		id, err := accountID(row[2])
		if err != nil {
			return deny()
		}
		if row[0] == "homenode-proxy" {
			if proxy != 0 || id < 100 || id > 999 || id == gid {
				return deny()
			}
			proxy = id
			members := strings.Split(row[3], ",")
			if len(members) != 2 || !((members[0] == "homenode" && members[1] == "homenode-gateway") || (members[1] == "homenode" && members[0] == "homenode-gateway")) {
				return deny()
			}
		} else {
			base.WriteString(strings.Join(row, ":"))
			base.WriteByte('\n')
		}
		if row[0] == "homenode-gateway" {
			if private || id != gid || (row[3] != "" && row[3] != "homenode-gateway") {
				return deny()
			}
			private = true
		} else if id == gid {
			return deny()
		}
		for _, member := range strings.Split(row[3], ",") {
			if member == "homenode-gateway" && row[0] != "homenode-gateway" && row[0] != "homenode-proxy" {
				return deny()
			}
		}
	}
	if !private || proxy == 0 {
		return deny()
	}
	for _, row := range groups {
		id, _ := accountID(row[2])
		if id == proxy && row[0] != "homenode-proxy" {
			return deny()
		}
	}
	for _, row := range users {
		otherUID, e1 := accountID(row[2])
		otherGID, e2 := accountID(row[3])
		if e1 != nil || e2 != nil || otherGID == proxy || (row[0] != "homenode-gateway" && (otherUID == uid || otherGID == gid)) {
			return deny()
		}
	}
	locked := false
	for _, row := range secrets {
		if row[0] == "homenode-gateway" {
			if locked {
				return deny()
			}
			locked = strings.HasPrefix(row[1], "!") || strings.HasPrefix(row[1], "*")
		}
	}
	if !locked {
		return deny()
	}
	accounts, err := validateBaseLocalAccounts(passwd, []byte(base.String()), shadow)
	if err != nil {
		return deny()
	}
	return accounts, GatewayAccount{UID: uint32(uid), GID: gid, ProxyGID: proxy}, nil
}
