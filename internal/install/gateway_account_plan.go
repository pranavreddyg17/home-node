package install

import (
	"encoding/hex"
	"strconv"
	"strings"
)

// GatewayAccount separates TLS key access from the controller's runtime groups.
// ProxyGID is a supplementary group shared only with the controller.
type GatewayAccount struct {
	UID      uint32 `json:"uid"`
	GID      int    `json:"gid"`
	ProxyGID int    `json:"proxyGid"`
}

type GatewayAccountPlan struct {
	OwnerID  string           `json:"ownerId"`
	Identity GatewayAccount   `json:"identity"`
	Commands []AccountCommand `json:"commands"`
}

// PlanGatewayAccountCreation is read-only. Executing this plan requires an
// ownership journal, live NSS vacancy checks and per-command verification.
// It deliberately refuses adoption, even for apparently suitable accounts.
func PlanGatewayAccountCreation(owner string, passwd, groups, shadow, nss []byte) (GatewayAccountPlan, error) {
	var empty GatewayAccountPlan
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
	reserved := func(name string) bool { return name == "homenode-gateway" || name == "homenode-proxy" }
	usedUID, usedGID := map[int]bool{}, map[int]bool{}
	for _, row := range users {
		if reserved(row[0]) {
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
		if reserved(row[0]) {
			return empty, ErrConflict
		}
		gid, err := accountID(row[2])
		if err != nil {
			return empty, err
		}
		usedGID[gid] = true
		for _, member := range strings.Split(row[3], ",") {
			if reserved(member) {
				return empty, ErrConflict
			}
		}
	}
	for _, row := range secrets {
		if reserved(row[0]) {
			return empty, ErrConflict
		}
	}
	reserve := func(used map[int]bool) int {
		for id := 999; id >= 100; id-- {
			if !used[id] {
				used[id] = true
				return id
			}
		}
		return 0
	}
	identity := GatewayAccount{UID: uint32(reserve(usedUID)), GID: reserve(usedGID), ProxyGID: reserve(usedGID)}
	if identity.UID == 0 || identity.GID == 0 || identity.ProxyGID == 0 {
		return empty, ErrAccounts
	}
	commands := []AccountCommand{
		{Program: "/usr/sbin/groupadd", Arguments: []string{"--system", "--gid", strconv.Itoa(identity.GID), "homenode-gateway"}},
		{Program: "/usr/sbin/groupadd", Arguments: []string{"--system", "--gid", strconv.Itoa(identity.ProxyGID), "homenode-proxy"}},
		{Program: "/usr/sbin/useradd", Arguments: []string{"--system", "--uid", strconv.FormatUint(uint64(identity.UID), 10), "--gid", "homenode-gateway", "--groups", "homenode-proxy", "--no-create-home", "--home-dir", "/nonexistent", "--shell", "/usr/sbin/nologin", "--comment", "HomeNode install " + owner, "homenode-gateway"}},
		{Program: "/usr/sbin/usermod", Arguments: []string{"--append", "--groups", "homenode-proxy", "homenode"}},
	}
	return GatewayAccountPlan{OwnerID: owner, Identity: identity, Commands: commands}, nil
}
