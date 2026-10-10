package install

import (
	"bytes"
	"errors"
	"strconv"
	"strings"
)

var ErrAccounts = errors.New("HomeNode requires distinct local service accounts with locked passwords, nologin shells and isolated groups")

const maxAccountFileBytes = 1 << 20

type localUser struct {
	name        string
	uid, gid    int
	home, shell string
	shadowed    bool
}
type localGroup struct {
	name    string
	gid     int
	members []string
}

func accountLines(data []byte, fields int) ([][]string, error) {
	if len(data) == 0 || len(data) > maxAccountFileBytes || bytes.IndexByte(data, 0) >= 0 {
		return nil, ErrAccounts
	}
	var rows [][]string
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" {
			continue
		}
		if len(line) > 64<<10 {
			return nil, ErrAccounts
		}
		row := strings.Split(line, ":")
		if len(row) != fields || row[0] == "" {
			return nil, ErrAccounts
		}
		rows = append(rows, row)
	}
	return rows, nil
}
func accountID(value string) (int, error) {
	id, err := strconv.ParseUint(value, 10, 31)
	if err != nil {
		return 0, ErrAccounts
	}
	return int(id), nil
}

// ValidateLocalAccounts evaluates bounded local passwd/group/shadow snapshots.
// It returns only numeric service identities, never password/hash content.
// Actual NSS resolution must agree before the production installer uses them.
func validateBaseLocalAccounts(passwd, group, shadow []byte) (Accounts, error) {
	var empty Accounts
	usersRaw, err := accountLines(passwd, 7)
	if err != nil {
		return empty, err
	}
	groupsRaw, err := accountLines(group, 4)
	if err != nil {
		return empty, err
	}
	shadowRaw, err := accountLines(shadow, 9)
	if err != nil {
		return empty, err
	}
	users := map[string]localUser{}
	for _, row := range usersRaw {
		if _, exists := users[row[0]]; exists {
			return empty, ErrAccounts
		}
		uid, err := accountID(row[2])
		if err != nil {
			return empty, err
		}
		gid, err := accountID(row[3])
		if err != nil {
			return empty, err
		}
		users[row[0]] = localUser{name: row[0], uid: uid, gid: gid, home: row[5], shell: row[6], shadowed: row[1] == "x"}
	}
	groups := map[string]localGroup{}
	for _, row := range groupsRaw {
		if _, exists := groups[row[0]]; exists {
			return empty, ErrAccounts
		}
		gid, err := accountID(row[2])
		if err != nil {
			return empty, err
		}
		var members []string
		if row[3] != "" {
			members = strings.Split(row[3], ",")
			for _, name := range members {
				if name == "" {
					return empty, ErrAccounts
				}
			}
		}
		groups[row[0]] = localGroup{row[0], gid, members}
	}
	locked := map[string]bool{}
	for _, row := range shadowRaw {
		if _, exists := locked[row[0]]; exists {
			return empty, ErrAccounts
		}
		locked[row[0]] = strings.HasPrefix(row[1], "!") || strings.HasPrefix(row[1], "*")
	}
	controller, ok := users["homenode"]
	if !ok {
		return empty, ErrAccounts
	}
	transfer, ok := users["homenode-transfer"]
	if !ok {
		return empty, ErrAccounts
	}
	for _, service := range []localUser{controller, transfer} {
		if service.uid == 0 || service.uid >= 1000 || !service.shadowed || service.home != "/nonexistent" || (service.shell != "/usr/sbin/nologin" && service.shell != "/sbin/nologin") || !locked[service.name] {
			return empty, ErrAccounts
		}
		for _, other := range users {
			if other.name != service.name && other.uid == service.uid {
				return empty, ErrAccounts
			}
		}
	}
	if controller.uid == transfer.uid {
		return empty, ErrAccounts
	}
	required := []string{"homenode", "homenode-transfer", "homenode-runtime", "libvirt-qemu"}
	gids := map[int]bool{}
	for _, name := range required {
		g, ok := groups[name]
		if !ok || g.gid == 0 || gids[g.gid] {
			return empty, ErrAccounts
		}
		gids[g.gid] = true
		for _, other := range groups {
			if other.name != name && other.gid == g.gid {
				return empty, ErrAccounts
			}
		}
	}
	if controller.gid >= 1000 || transfer.gid >= 1000 || groups["homenode-runtime"].gid >= 1000 {
		return empty, ErrAccounts
	}
	if controller.gid != groups["homenode"].gid || transfer.gid != groups["homenode-transfer"].gid {
		return empty, ErrAccounts
	}
	allowed := map[string]map[int]bool{
		"homenode":          {controller.gid: true, groups["homenode-runtime"].gid: true},
		"homenode-transfer": {transfer.gid: true, groups["homenode-runtime"].gid: true},
	}
	runtimeMembers := map[string]bool{}
	for _, g := range groups {
		for _, name := range g.members {
			if expected, service := allowed[name]; service && !expected[g.gid] {
				return empty, ErrAccounts
			}
			if g.name == "homenode" && name != "homenode" {
				return empty, ErrAccounts
			}
			if g.name == "homenode-transfer" && name != "homenode-transfer" {
				return empty, ErrAccounts
			}
			if g.name == "homenode-runtime" {
				if name != "homenode" && name != "homenode-transfer" {
					return empty, ErrAccounts
				}
				runtimeMembers[name] = true
			}
		}
	}
	if !runtimeMembers[controller.name] || !runtimeMembers[transfer.name] {
		return empty, ErrAccounts
	}
	for _, u := range users {
		if (u.gid == controller.gid && u.name != controller.name) || (u.gid == transfer.gid && u.name != transfer.name) || u.gid == groups["homenode-runtime"].gid {
			return empty, ErrAccounts
		}
	}
	return Accounts{ControllerUID: uint32(controller.uid), TransferUID: uint32(transfer.uid), ControllerGID: controller.gid, TransferGID: transfer.gid, RuntimeGID: groups["homenode-runtime"].gid, QEMUGID: groups["libvirt-qemu"].gid}, nil
}

// Initial supported hosts use local files (plus systemd's local resolver).
// Remote identity providers require a separate UID/PAM isolation review.
func ValidateNameServices(data []byte) error {
	if len(data) == 0 || len(data) > maxAccountFileBytes || bytes.IndexByte(data, 0) >= 0 {
		return ErrAccounts
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		line, _, _ = strings.Cut(line, "#")
		name, body, ok := strings.Cut(line, ":")
		if !ok {
			if strings.TrimSpace(line) != "" {
				return ErrAccounts
			}
			continue
		}
		name = strings.TrimSpace(name)
		if name != "passwd" && name != "group" && name != "shadow" {
			continue
		}
		if seen[name] {
			return ErrAccounts
		}
		seen[name] = true
		sources := strings.Fields(body)
		if len(sources) == 0 || len(sources) > 2 || sources[0] != "files" || (len(sources) == 2 && sources[1] != "systemd") {
			return ErrAccounts
		}
	}
	if !seen["passwd"] || !seen["group"] || !seen["shadow"] {
		return ErrAccounts
	}
	return nil
}
