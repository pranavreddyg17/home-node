package install

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func readAccountFile(path string, private bool) ([]byte, error) {
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() {
		return nil, ErrAccounts
	}
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, ErrAccounts
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !os.SameFile(before, info) || !info.Mode().IsRegular() || !owned(info, 0) || info.Mode().Perm()&0022 != 0 || (private && info.Mode().Perm()&0007 != 0) {
		return nil, ErrAccounts
	}
	data, err := io.ReadAll(io.LimitReader(file, maxAccountFileBytes+1))
	if err != nil || len(data) > maxAccountFileBytes {
		return nil, ErrAccounts
	}
	return data, nil
}

// InspectLocalAccounts reads root-controlled local databases and confirms that
// name resolution/group membership agrees. It cannot configure remote NSS/PAM.
func InspectLocalAccounts(ctx context.Context) (Accounts, error) {
	var empty Accounts
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return empty, fmt.Errorf("account inspection requires Linux and root: %w", ErrAccounts)
	}
	nss, err := readAccountFile("/etc/nsswitch.conf", false)
	if err != nil || ValidateNameServices(nss) != nil {
		return empty, ErrAccounts
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
	accounts, err := ValidateLocalAccounts(passwd, groups, shadow)
	if err != nil {
		return empty, err
	}
	var gateway *GatewayAccount
	groupRows, err := accountLines(groups, 4)
	if err != nil {
		return empty, err
	}
	for _, row := range groupRows {
		if row[0] == "homenode-proxy" {
			identity, err := ValidateGatewayAccount(passwd, groups, shadow)
			if err != nil {
				return empty, err
			}
			gateway = &identity
		}
	}
	if err = resolvedAccounts(ctx, accounts, gateway); err != nil {
		return empty, err
	}
	return accounts, nil
}

type accountOutput struct{ bytes.Buffer }

func (b *accountOutput) Write(data []byte) (int, error) {
	if b.Len()+len(data) > 8<<10 {
		return 0, ErrAccounts
	}
	return b.Buffer.Write(data)
}
func accountCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	deadline, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(deadline, name, args...)
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "LC_ALL=C"}
	cmd.WaitDelay = time.Second
	var output accountOutput
	cmd.Stdout = &output
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return nil, ErrAccounts
	}
	return output.Bytes(), nil
}
func resolvedAccounts(ctx context.Context, a Accounts, gateway *GatewayAccount) error {
	return resolvedAccountsWith(ctx, a, gateway, accountCommand)
}

func resolvedAccountsWith(ctx context.Context, a Accounts, gateway *GatewayAccount, command func(context.Context, string, ...string) ([]byte, error)) error {
	deadline, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	type userEntry struct {
		name   string
		uid    uint32
		gid    int
		groups []int
	}
	users := []userEntry{{"homenode", a.ControllerUID, a.ControllerGID, []int{a.ControllerGID, a.RuntimeGID}}, {"homenode-transfer", a.TransferUID, a.TransferGID, []int{a.TransferGID, a.RuntimeGID}}}
	if gateway != nil {
		users[0].groups = append(users[0].groups, gateway.ProxyGID)
		users = append(users, userEntry{"homenode-gateway", gateway.UID, gateway.GID, []int{gateway.GID, gateway.ProxyGID}})
	}
	for _, entry := range users {
		data, err := command(deadline, "/usr/bin/getent", "passwd", entry.name)
		if err != nil {
			return ErrAccounts
		}
		rows, err := accountLines(data, 7)
		if err != nil || len(rows) != 1 || rows[0][0] != entry.name || rows[0][2] != strconv.FormatUint(uint64(entry.uid), 10) || rows[0][3] != strconv.Itoa(entry.gid) || rows[0][5] != "/nonexistent" || (rows[0][6] != "/usr/sbin/nologin" && rows[0][6] != "/sbin/nologin") {
			return ErrAccounts
		}
		data, err = command(deadline, "/usr/bin/id", "-G", entry.name)
		if err != nil {
			return ErrAccounts
		}
		expected := map[string]bool{}
		for _, group := range entry.groups {
			expected[strconv.Itoa(group)] = false
		}
		for _, id := range strings.Fields(string(data)) {
			if _, ok := expected[id]; !ok {
				return ErrAccounts
			}
			expected[id] = true
		}
		for _, found := range expected {
			if !found {
				return ErrAccounts
			}
		}
	}
	type groupEntry struct {
		name string
		gid  int
	}
	groups := []groupEntry{{"homenode", a.ControllerGID}, {"homenode-transfer", a.TransferGID}, {"homenode-runtime", a.RuntimeGID}, {"libvirt-qemu", a.QEMUGID}}
	if gateway != nil {
		groups = append(groups, groupEntry{"homenode-gateway", gateway.GID}, groupEntry{"homenode-proxy", gateway.ProxyGID})
	}
	for _, entry := range groups {
		data, err := command(deadline, "/usr/bin/getent", "group", entry.name)
		if err != nil {
			return ErrAccounts
		}
		rows, err := accountLines(data, 4)
		if err != nil || len(rows) != 1 || rows[0][0] != entry.name || rows[0][2] != strconv.Itoa(entry.gid) {
			return ErrAccounts
		}
	}
	return nil
}

// lookupAccount distinguishes a definite getent "not found" result from a
// timeout, unavailable resolver or malformed response. Cleanup may skip only
// definite absence; other errors remain actionable.
func lookupAccount(ctx context.Context, database, key string) ([]byte, bool, error) {
	if database != "passwd" && database != "group" {
		return nil, false, ErrAccounts
	}
	allowed := key == "homenode" || key == "homenode-transfer" || key == "homenode-runtime" || key == "homenode-backup" || key == "homenode-gateway" || key == "homenode-proxy" || key == "libvirt-qemu"
	if !allowed {
		if _, err := accountID(key); err != nil {
			return nil, false, ErrAccounts
		}
	}
	deadline, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(deadline, "/usr/bin/getent", database, key)
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "LC_ALL=C"}
	cmd.WaitDelay = time.Second
	var output accountOutput
	cmd.Stdout = &output
	cmd.Stderr = io.Discard
	err := cmd.Run()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 2 && output.Len() == 0 && deadline.Err() == nil {
			return nil, false, nil
		}
		return nil, false, ErrAccounts
	}
	if output.Len() == 0 {
		return nil, false, ErrAccounts
	}
	return output.Bytes(), true, nil
}
