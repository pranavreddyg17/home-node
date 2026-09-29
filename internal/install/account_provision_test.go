package install

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeAccountProvisioner struct {
	s        accountSnapshot
	commands int
	collide  bool
}

func newAccountFixture() *fakeAccountProvisioner {
	return &fakeAccountProvisioner{s: accountSnapshot{passwd: []byte("root:x:0:0:root:/root:/bin/bash\n"), groups: []byte("root:x:0:\nlibvirt-qemu:x:64055:libvirt-qemu\n"), shadow: []byte("root:!:1:0:99999:7:::\n"), nss: []byte("passwd: files\ngroup: files\nshadow: files\n")}}
}
func (b *fakeAccountProvisioner) Snapshot(context.Context) (accountSnapshot, error) { return b.s, nil }
func (b *fakeAccountProvisioner) Lookup(_ context.Context, db, key string) (bool, error) {
	if b.collide {
		return true, nil
	}
	data, fields, id := b.s.groups, 4, 2
	if db == "passwd" {
		data, fields, id = b.s.passwd, 7, 2
	}
	rows, err := accountLines(data, fields)
	if err != nil {
		return false, err
	}
	for _, row := range rows {
		if row[0] == key || row[id] == key {
			return true, nil
		}
	}
	return false, nil
}
func (b *fakeAccountProvisioner) Execute(_ context.Context, c AccountCommand) error {
	b.commands++
	name := c.Arguments[len(c.Arguments)-1]
	if c.Program == "/usr/sbin/groupadd" {
		b.s.groups = append(b.s.groups, []byte(name+":x:"+c.Arguments[2]+":\n")...)
		return nil
	}
	groups, err := accountLines(b.s.groups, 4)
	if err != nil {
		return err
	}
	gid := ""
	for _, row := range groups {
		if row[0] == name {
			gid = row[2]
		}
		if row[0] == "homenode-runtime" {
			if row[3] != "" {
				row[3] += ","
			}
			row[3] += name
		}
	}
	if gid == "" {
		return ErrAccounts
	}
	var lines []string
	for _, row := range groups {
		lines = append(lines, strings.Join(row, ":"))
	}
	b.s.groups = []byte(strings.Join(lines, "\n") + "\n")
	marker := c.Arguments[len(c.Arguments)-2]
	b.s.passwd = append(b.s.passwd, []byte(name+":x:"+c.Arguments[2]+":"+gid+":"+marker+":/nonexistent:/usr/sbin/nologin\n")...)
	b.s.shadow = append(b.s.shadow, []byte(name+":!:1:0:99999:7:::\n")...)
	return nil
}
func (b *fakeAccountProvisioner) Verify(context.Context) (Accounts, error) {
	return ValidateLocalAccounts(b.s.passwd, b.s.groups, b.s.shadow)
}

func TestAccountProvisionReconcilesCreatedIdentityAfterLostResult(t *testing.T) {
	host, jr := roots(t)
	engine := openEngine(t, host, jr)
	backend := newAccountFixture()
	engine.checkpoint = func(stage, name string) error {
		if stage == "account-created" && backend.commands == 4 {
			return errors.New("process stopped before recording command result")
		}
		return nil
	}
	if _, err := engine.provisionAccounts(context.Background(), backend); err == nil {
		t.Fatal("interruption hidden")
	}
	j, err := engine.loadAccountJournal()
	if err != nil || j.Completed != 3 {
		t.Fatal("intent not retained", j.Completed, err)
	}
	if err = engine.Close(); err != nil {
		t.Fatal(err)
	}
	engine = openEngine(t, host, jr)
	defer engine.Close()
	actual, err := engine.provisionAccounts(context.Background(), backend)
	if err != nil || actual != j.Accounts || backend.commands != 5 {
		t.Fatal("created user duplicated or lost", actual, backend.commands, err)
	}
	if _, err = engine.provisionAccounts(context.Background(), backend); err != nil || backend.commands != 5 {
		t.Fatal("ready provision replayed commands", err)
	}
}
func TestAccountProvisionPreservesCollisionsAndChangedOwnership(t *testing.T) {
	host, jr := roots(t)
	engine := openEngine(t, host, jr)
	defer engine.Close()
	backend := newAccountFixture()
	backend.collide = true
	if _, err := engine.provisionAccounts(context.Background(), backend); err == nil || backend.commands != 0 {
		t.Fatal("live namespace collision overwritten", err)
	}
	backend.collide = false
	if _, err := engine.provisionAccounts(context.Background(), backend); err != nil {
		t.Fatal(err)
	}
	before := backend.commands
	backend.s.passwd = []byte(strings.Replace(string(backend.s.passwd), "HomeNode install ", "Foreign owner ", 1))
	if _, err := engine.provisionAccounts(context.Background(), backend); err == nil || backend.commands != before {
		t.Fatal("changed account adopted or repaired", err)
	}
}
func TestAccountProvisionRefusesForeignInitialNames(t *testing.T) {
	host, jr := roots(t)
	engine := openEngine(t, host, jr)
	defer engine.Close()
	backend := newAccountFixture()
	backend.s.groups = append(backend.s.groups, []byte("homenode:x:800:\n")...)
	if _, err := engine.provisionAccounts(context.Background(), backend); err == nil || backend.commands != 0 {
		t.Fatal("foreign group adopted", err)
	}
	if _, err := engine.loadAccountJournal(); err == nil {
		t.Fatal("foreign ownership intent committed")
	}
}

func TestCompletedAccountDisappearanceDoesNotReuseUID(t *testing.T) {
	host, jr := roots(t)
	engine := openEngine(t, host, jr)
	defer engine.Close()
	backend := newAccountFixture()
	if _, err := engine.provisionAccounts(context.Background(), backend); err != nil {
		t.Fatal(err)
	}
	before := backend.commands
	users, _ := accountLines(backend.s.passwd, 7)
	var lines []string
	for _, row := range users {
		if row[0] != "homenode-transfer" {
			lines = append(lines, strings.Join(row, ":"))
		}
	}
	backend.s.passwd = []byte(strings.Join(lines, "\n") + "\n")
	backend.s.groups = []byte(strings.Replace(string(backend.s.groups), ",homenode-transfer", "", 1))
	if _, err := engine.provisionAccounts(context.Background(), backend); err == nil || backend.commands != before {
		t.Fatal("missing completed identity recreated", err)
	}
}
