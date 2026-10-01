package install

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
)

type fixtureMaintenanceProvisioner struct{ *fakeAccountProvisioner }

func (b fixtureMaintenanceProvisioner) Execute(ctx context.Context, c AccountCommand) error {
	if c.Program == "/usr/sbin/groupadd" {
		return b.fakeAccountProvisioner.Execute(ctx, c)
	}
	b.commands++
	rows, err := accountLines(b.s.groups, 4)
	if err != nil {
		return err
	}
	gid := ""
	for _, row := range rows {
		if row[0] == "homenode-backup" {
			gid = row[2]
		}
	}
	if gid == "" {
		return ErrConflict
	}
	marker := c.Arguments[len(c.Arguments)-2]
	b.s.passwd = append(b.s.passwd, []byte("homenode-backup:x:"+c.Arguments[2]+":"+gid+":"+marker+":/nonexistent:/usr/sbin/nologin\n")...)
	b.s.shadow = append(b.s.shadow, []byte("homenode-backup:!:1:0:99999:7:::\n")...)
	return nil
}
func (b fixtureMaintenanceProvisioner) VerifyMaintenance(context.Context) (MaintenanceAccount, error) {
	return ValidateMaintenanceAccount(b.s.passwd, b.s.groups, b.s.shadow)
}

func TestMaintenanceProvisionResumesLostCommandResult(t *testing.T) {
	for _, interruption := range []int{6, 7} {
		t.Run(strconv.Itoa(interruption), func(t *testing.T) {
			host, journal := roots(t)
			engine := openEngine(t, host, journal)
			base := newAccountFixture()
			ctx := context.Background()
			if _, err := engine.provisionAccounts(ctx, base); err != nil {
				t.Fatal(err)
			}
			plan, err := engine.prepareMaintenanceAccount(ctx, base)
			if err != nil {
				t.Fatal(err)
			}
			backend := fixtureMaintenanceProvisioner{base}
			stopped := errors.New("fixture stopped after mutation")
			engine.checkpoint = func(stage, name string) error {
				if stage == "maintenance-account-created" && base.commands == interruption {
					return stopped
				}
				return nil
			}
			if _, err = engine.provisionMaintenanceAccount(ctx, backend); !errors.Is(err, stopped) {
				t.Fatal("lost result hidden", err)
			}
			if err = engine.Close(); err != nil {
				t.Fatal(err)
			}
			engine = openEngine(t, host, journal)
			defer engine.Close()
			actual, err := engine.provisionMaintenanceAccount(ctx, backend)
			if err != nil || actual != plan.Identity || base.commands != 7 {
				t.Fatal("mutation replayed or identity lost", actual, base.commands, err)
			}
			if _, err = engine.provisionMaintenanceAccount(ctx, backend); err != nil || base.commands != 7 {
				t.Fatal("ready replay changed account", base.commands, err)
			}
			baseJournal, err := engine.loadAccountJournal()
			if err != nil {
				t.Fatal(err)
			}
			j, err := engine.loadMaintenanceAccountJournal(baseJournal)
			if err != nil || !j.Ready || j.Completed != 2 {
				t.Fatal("readiness not durable", j, err)
			}
			base.s.groups = []byte(strings.Replace(string(base.s.groups), "homenode-backup:x:"+strconv.Itoa(plan.Identity.GID)+":\n", "", 1))
			if _, err = engine.provisionMaintenanceAccount(ctx, backend); err == nil || base.commands != 7 {
				t.Fatal("completed missing group recreated", base.commands, err)
			}
		})
	}
}

func TestMaintenanceProvisionRequiresPreparedIntentAndVacancy(t *testing.T) {
	host, journal := roots(t)
	engine := openEngine(t, host, journal)
	defer engine.Close()
	base := newAccountFixture()
	ctx := context.Background()
	if _, err := engine.provisionAccounts(ctx, base); err != nil {
		t.Fatal(err)
	}
	backend := fixtureMaintenanceProvisioner{base}
	if _, err := engine.provisionMaintenanceAccount(ctx, backend); err == nil || base.commands != 5 {
		t.Fatal("unprepared mutation", base.commands, err)
	}
	if _, err := engine.prepareMaintenanceAccount(ctx, base); err != nil {
		t.Fatal(err)
	}
	base.collide = true
	if _, err := engine.provisionMaintenanceAccount(ctx, backend); !errors.Is(err, ErrConflict) || base.commands != 5 {
		t.Fatal("live collision mutated account", base.commands, err)
	}
	base.collide = false
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := engine.provisionMaintenanceAccount(canceled, backend); !errors.Is(err, context.Canceled) || base.commands != 5 {
		t.Fatal("canceled mutation", base.commands, err)
	}
}
