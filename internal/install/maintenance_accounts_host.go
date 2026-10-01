package install

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// InspectMaintenanceAccount performs no mutation. Local account snapshots and
// live NSS resolution must agree before this identity can configure a socket.
func InspectMaintenanceAccount(ctx context.Context) (MaintenanceAccount, error) {
	var empty MaintenanceAccount
	deadline, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	nss, err := readAccountFile("/etc/nsswitch.conf", false)
	if err != nil {
		return empty, err
	}
	if err = ValidateNameServices(nss); err != nil {
		return empty, err
	}
	passwd, err := readAccountFile("/etc/passwd", false)
	if err != nil {
		return empty, err
	}
	groups, err := readAccountFile("/etc/group", false)
	if err != nil {
		return empty, err
	}
	shadow, err := readAccountFile("/etc/shadow", true)
	if err != nil {
		return empty, err
	}
	defer clear(shadow)
	identity, err := ValidateMaintenanceAccount(passwd, groups, shadow)
	if err != nil {
		return empty, err
	}
	data, err := accountCommand(deadline, "/usr/bin/getent", "passwd", "homenode-backup")
	if err != nil {
		return empty, ErrAccounts
	}
	rows, err := accountLines(data, 7)
	if err != nil || len(rows) != 1 || rows[0][0] != "homenode-backup" || rows[0][2] != strconv.FormatUint(uint64(identity.UID), 10) || rows[0][3] != strconv.Itoa(identity.GID) || rows[0][5] != "/nonexistent" || (rows[0][6] != "/usr/sbin/nologin" && rows[0][6] != "/sbin/nologin") {
		return empty, ErrAccounts
	}
	data, err = accountCommand(deadline, "/usr/bin/getent", "group", "homenode-backup")
	if err != nil {
		return empty, ErrAccounts
	}
	rows, err = accountLines(data, 4)
	if err != nil || len(rows) != 1 || rows[0][0] != "homenode-backup" || rows[0][2] != strconv.Itoa(identity.GID) || (rows[0][3] != "" && rows[0][3] != "homenode-backup") {
		return empty, ErrAccounts
	}
	data, err = accountCommand(deadline, "/usr/bin/id", "-G", "homenode-backup")
	if err != nil {
		return empty, ErrAccounts
	}
	membership := strings.Fields(string(data))
	if len(membership) != 1 || membership[0] != strconv.Itoa(identity.GID) {
		return empty, ErrAccounts
	}
	return identity, nil
}
