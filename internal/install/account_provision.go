package install

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

type accountSnapshot struct{ passwd, groups, shadow, nss []byte }
type accountProvisionBackend interface {
	Snapshot(context.Context) (accountSnapshot, error)
	Lookup(context.Context, string, string) (bool, error)
	Execute(context.Context, AccountCommand) error
	Verify(context.Context) (Accounts, error)
}
type accountJournal struct {
	Version   int      `json:"version"`
	OwnerID   string   `json:"ownerId"`
	Accounts  Accounts `json:"accounts"`
	Digest    string   `json:"digest"`
	Completed int      `json:"completed"`
	Ready     bool     `json:"ready"`
}

func accountIntentDigest(j accountJournal) string {
	data, _ := json.Marshal(struct {
		OwnerID  string
		Accounts Accounts
	}{j.OwnerID, j.Accounts})
	return digest(data)
}
func (e *Engine) loadAccountJournal() (accountJournal, error) {
	var j accountJournal
	file, err := e.journalRoot.OpenFile("accounts.json", os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return j, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || !owned(info, e.owner) || info.Mode().Perm() != 0600 {
		return j, ErrConflict
	}
	decoder := json.NewDecoder(io.LimitReader(file, (64<<10)+1))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&j) != nil || decoder.Decode(new(any)) != io.EOF {
		return j, ErrConflict
	}
	a := j.Accounts
	if j.Version != 1 || len(j.OwnerID) != 32 || j.Completed < 0 || j.Completed > 5 || (j.Ready && j.Completed != 5) || j.Digest != accountIntentDigest(j) {
		return j, ErrConflict
	}
	if _, err = hex.DecodeString(j.OwnerID); err != nil {
		return j, ErrConflict
	}
	if a.ControllerUID < 100 || a.ControllerUID >= 1000 || a.TransferUID < 100 || a.TransferUID >= 1000 || a.ControllerUID == a.TransferUID {
		return j, ErrConflict
	}
	gids := map[int]bool{}
	for _, gid := range []int{a.ControllerGID, a.TransferGID, a.RuntimeGID} {
		if gid < 100 || gid >= 1000 || gids[gid] {
			return j, ErrConflict
		}
		gids[gid] = true
	}
	if a.QEMUGID < 1 || a.QEMUGID > 1<<31-1 || gids[a.QEMUGID] {
		return j, ErrConflict
	}
	return j, nil
}
func (e *Engine) saveAccountJournal(j accountJournal) error {
	data, err := json.Marshal(j)
	if err != nil {
		return err
	}
	return e.saveJournalBytes("accounts", data)
}

// ProvisionAccounts is the root-only account phase. The private journal must
// already exist; it shares the configuration installer lock. No service starts
// and no account deletion occurs here.
func (e *Engine) ProvisionAccounts(ctx context.Context) (Accounts, error) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 || e.host.Name() != "/" {
		return Accounts{}, ErrAccounts
	}
	return e.provisionAccounts(ctx, nativeAccountProvisioner{})
}
func (e *Engine) provisionAccounts(ctx context.Context, b accountProvisionBackend) (Accounts, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	var empty Accounts
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	j, err := e.loadAccountJournal()
	if os.IsNotExist(err) {
		snapshot, err := b.Snapshot(ctx)
		if err != nil {
			return empty, err
		}
		owner, err := newID()
		if err != nil {
			return empty, err
		}
		plan, err := PlanAccountCreation(owner, snapshot.passwd, snapshot.groups, snapshot.nss)
		if err != nil {
			return empty, err
		}
		j = accountJournal{Version: 1, OwnerID: owner, Accounts: plan.Accounts}
		j.Digest = accountIntentDigest(j)
		if err = e.saveAccountJournal(j); err != nil {
			return empty, err
		}
	} else if err != nil {
		return empty, err
	}
	commands := creationCommands(j.OwnerID, j.Accounts)
	for index, command := range commands {
		if err = ctx.Err(); err != nil {
			return empty, err
		}
		snapshot, err := b.Snapshot(ctx)
		if err != nil {
			return empty, err
		}
		present, err := accountStepMatches(snapshot, j, index)
		if err != nil {
			return empty, err
		}
		if !present {
			if index < j.Completed {
				return empty, ErrConflict
			}
			database, key, id := "group", command.Arguments[len(command.Arguments)-1], command.Arguments[2]
			if index >= 3 {
				database = "passwd"
			}
			for _, value := range []string{key, id} {
				exists, err := b.Lookup(ctx, database, value)
				if err != nil {
					return empty, err
				}
				if exists {
					return empty, ErrConflict
				}
			}
			if err = b.Execute(ctx, command); err != nil {
				return empty, err
			}
			snapshot, err = b.Snapshot(ctx)
			if err != nil {
				return empty, err
			}
			present, err = accountStepMatches(snapshot, j, index)
			if err != nil || !present {
				return empty, ErrConflict
			}
			if e.checkpoint != nil {
				if err = e.checkpoint("account-created", key); err != nil {
					return empty, err
				}
			}
		}
		if index >= j.Completed {
			j.Completed = index + 1
			if err = e.saveAccountJournal(j); err != nil {
				return empty, err
			}
		}
	}
	actual, err := b.Verify(ctx)
	if err != nil {
		return empty, err
	}
	if actual != j.Accounts {
		return empty, ErrConflict
	}
	j.Ready = true
	if err = e.saveAccountJournal(j); err != nil {
		return empty, err
	}
	return actual, nil
}
func accountStepMatches(s accountSnapshot, j accountJournal, index int) (bool, error) {
	if ValidateNameServices(s.nss) != nil {
		return false, ErrAccounts
	}
	users, err := accountLines(s.passwd, 7)
	if err != nil {
		return false, err
	}
	groups, err := accountLines(s.groups, 4)
	if err != nil {
		return false, err
	}
	shadow, err := accountLines(s.shadow, 9)
	if err != nil {
		return false, err
	}
	a := j.Accounts
	userExpected := map[string]struct{ uid, gid int }{"homenode": {int(a.ControllerUID), a.ControllerGID}, "homenode-transfer": {int(a.TransferUID), a.TransferGID}}
	ownedUsers := map[string]bool{}
	for name, expected := range userExpected {
		found := false
		for _, row := range users {
			uid, e1 := accountID(row[2])
			gid, e2 := accountID(row[3])
			if e1 != nil || e2 != nil {
				return false, ErrAccounts
			}
			if row[0] != name {
				if uid == expected.uid {
					return false, ErrConflict
				}
				continue
			}
			if found || uid != expected.uid || gid != expected.gid || row[1] != "x" || row[4] != "HomeNode install "+j.OwnerID || row[5] != "/nonexistent" || row[6] != "/usr/sbin/nologin" {
				return false, ErrConflict
			}
			found = true
		}
		if found {
			locked, count := false, 0
			for _, row := range shadow {
				if row[0] == name {
					count++
					locked = strings.HasPrefix(row[1], "!") || strings.HasPrefix(row[1], "*")
				}
			}
			if count != 1 || !locked {
				return false, ErrConflict
			}
			ownedUsers[name] = true
		}
	}
	groupExpected := map[string]int{"homenode": a.ControllerGID, "homenode-transfer": a.TransferGID, "homenode-runtime": a.RuntimeGID}
	foundGroups := map[string]bool{}
	runtimeMembers := map[string]bool{}
	for _, row := range groups {
		gid, err := accountID(row[2])
		if err != nil {
			return false, err
		}
		for name, expected := range groupExpected {
			if gid == expected && row[0] != name {
				return false, ErrConflict
			}
		}
		if expected, managed := groupExpected[row[0]]; managed {
			if foundGroups[row[0]] || gid != expected {
				return false, ErrConflict
			}
			foundGroups[row[0]] = true
			if row[3] != "" {
				for _, member := range strings.Split(row[3], ",") {
					if !ownedUsers[member] || (row[0] != "homenode-runtime" && member != row[0]) {
						return false, ErrConflict
					}
					if row[0] == "homenode-runtime" {
						runtimeMembers[member] = true
					}
				}
			}
		} else {
			for _, member := range strings.Split(row[3], ",") {
				if _, service := userExpected[member]; service {
					return false, ErrConflict
				}
			}
		}
	}
	for _, row := range users {
		gid, err := accountID(row[3])
		if err != nil {
			return false, err
		}
		for name, expected := range groupExpected {
			if gid == expected && (name == "homenode-runtime" || row[0] != name) {
				return false, ErrConflict
			}
		}
	}
	names := []string{"homenode", "homenode-transfer", "homenode-runtime", "homenode", "homenode-transfer"}
	if index < 3 {
		return foundGroups[names[index]], nil
	}
	name := names[index]
	if ownedUsers[name] && !runtimeMembers[name] {
		return false, ErrConflict
	}
	return ownedUsers[name], nil
}

type nativeAccountProvisioner struct{}

func (nativeAccountProvisioner) Snapshot(ctx context.Context) (accountSnapshot, error) {
	var s accountSnapshot
	for _, item := range []struct {
		path    string
		private bool
		target  *[]byte
	}{{"/etc/passwd", false, &s.passwd}, {"/etc/group", false, &s.groups}, {"/etc/shadow", true, &s.shadow}, {"/etc/nsswitch.conf", false, &s.nss}} {
		if err := ctx.Err(); err != nil {
			return s, err
		}
		data, err := readAccountFile(item.path, item.private)
		if err != nil {
			return s, err
		}
		*item.target = data
	}
	return s, nil
}
func (nativeAccountProvisioner) Lookup(ctx context.Context, db, key string) (bool, error) {
	_, exists, err := lookupAccount(ctx, db, key)
	return exists, err
}
func (nativeAccountProvisioner) Execute(ctx context.Context, c AccountCommand) error {
	_, err := accountCommand(ctx, c.Program, c.Arguments...)
	return err
}
func (nativeAccountProvisioner) Verify(ctx context.Context) (Accounts, error) {
	return InspectLocalAccounts(ctx)
}

func creationCommands(owner string, a Accounts) []AccountCommand {
	var commands []AccountCommand
	for _, g := range []struct {
		name string
		gid  int
	}{{"homenode", a.ControllerGID}, {"homenode-transfer", a.TransferGID}, {"homenode-runtime", a.RuntimeGID}} {
		commands = append(commands, AccountCommand{"/usr/sbin/groupadd", []string{"--system", "--gid", strconv.Itoa(g.gid), g.name}})
	}
	for _, u := range []struct {
		name string
		uid  uint32
	}{{"homenode", a.ControllerUID}, {"homenode-transfer", a.TransferUID}} {
		commands = append(commands, AccountCommand{"/usr/sbin/useradd", []string{"--system", "--uid", strconv.FormatUint(uint64(u.uid), 10), "--gid", u.name, "--groups", "homenode-runtime", "--no-create-home", "--home-dir", "/nonexistent", "--shell", "/usr/sbin/nologin", "--comment", "HomeNode install " + owner, u.name}})
	}
	return commands
}
