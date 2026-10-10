package install

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
)

type fixtureGatewayProvisioner struct{ *fakeAccountProvisioner }

func (b fixtureGatewayProvisioner) Execute(ctx context.Context, c AccountCommand) error {
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
		if row[0] == "homenode-gateway" {
			gid = row[2]
		}
	}
	if gid == "" {
		return ErrConflict
	}
	if c.Program == "/usr/sbin/useradd" {
		b.s.passwd = append(b.s.passwd, []byte("homenode-gateway:x:"+c.Arguments[2]+":"+gid+":"+c.Arguments[len(c.Arguments)-2]+":/nonexistent:/usr/sbin/nologin\n")...)
		b.s.shadow = append(b.s.shadow, []byte("homenode-gateway:!:1:0:99999:7:::\n")...)
	}
	var group strings.Builder
	for _, row := range rows {
		if row[0] == "homenode-proxy" {
			if c.Program == "/usr/sbin/useradd" {
				row[3] = "homenode-gateway"
			} else {
				row[3] = "homenode,homenode-gateway"
			}
		}
		group.WriteString(strings.Join(row, ":"))
		group.WriteByte('\n')
	}
	b.s.groups = []byte(group.String())
	return nil
}
func (b fixtureGatewayProvisioner) VerifyGateway(context.Context) (GatewayAccount, error) {
	return ValidateGatewayAccount(b.s.passwd, b.s.groups, b.s.shadow)
}

func TestGatewayProvisionResumesEveryLostAcknowledgement(t *testing.T) {
	for _, interruption := range []int{6, 7, 8, 9} {
		t.Run(strconv.Itoa(interruption), func(t *testing.T) {
			host, journal := roots(t)
			engine := openEngine(t, host, journal)
			base := newAccountFixture()
			ctx := context.Background()
			if _, err := engine.provisionAccounts(ctx, base); err != nil {
				t.Fatal(err)
			}
			plan, err := engine.prepareGatewayAccount(ctx, base)
			if err != nil {
				t.Fatal(err)
			}
			backend := fixtureGatewayProvisioner{base}
			stopped := errors.New("lost acknowledgement")
			engine.checkpoint = func(stage, name string) error {
				if stage == "gateway-account-created" && base.commands == interruption {
					return stopped
				}
				return nil
			}
			if _, err := engine.provisionGatewayAccount(ctx, backend); !errors.Is(err, stopped) {
				t.Fatal("lost acknowledgement hidden", err)
			}
			engine.Close()
			engine = openEngine(t, host, journal)
			defer engine.Close()
			actual, err := engine.provisionGatewayAccount(ctx, backend)
			if err != nil || actual != plan.Identity || base.commands != 9 {
				t.Fatal("command replay or identity mismatch", actual, base.commands, err)
			}
			if _, err := engine.provisionGatewayAccount(ctx, backend); err != nil || base.commands != 9 {
				t.Fatal("ready replay mutated accounts", err)
			}
			j, err := engine.loadAccountJournal()
			if err != nil {
				t.Fatal(err)
			}
			gateway, err := engine.loadGatewayAccountJournal(j)
			if err != nil || !gateway.Ready || gateway.Completed != 4 {
				t.Fatal("readiness not persisted", gateway, err)
			}
			base.s.groups = []byte(strings.Replace(string(base.s.groups), "homenode,homenode-gateway", "homenode-gateway", 1))
			if _, err := engine.provisionGatewayAccount(ctx, backend); err == nil || base.commands != 9 {
				t.Fatal("missing completed membership silently recreated", err)
			}
		})
	}
}

func TestGatewayProvisionRequiresIntentVacancyAndUnchangedBaseOwner(t *testing.T) {
	host, journal := roots(t)
	engine := openEngine(t, host, journal)
	defer engine.Close()
	base := newAccountFixture()
	ctx := context.Background()
	if _, err := engine.provisionAccounts(ctx, base); err != nil {
		t.Fatal(err)
	}
	backend := fixtureGatewayProvisioner{base}
	if _, err := engine.provisionGatewayAccount(ctx, backend); err == nil || base.commands != 5 {
		t.Fatal("unprepared mutation", err)
	}
	if _, err := engine.prepareGatewayAccount(ctx, base); err != nil {
		t.Fatal(err)
	}
	base.collide = true
	if _, err := engine.provisionGatewayAccount(ctx, backend); !errors.Is(err, ErrConflict) || base.commands != 5 {
		t.Fatal("NSS collision mutated accounts", err)
	}
	base.collide = false
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := engine.provisionGatewayAccount(canceled, backend); !errors.Is(err, context.Canceled) || base.commands != 5 {
		t.Fatal("canceled mutation", err)
	}
	base.s.passwd = []byte(strings.Replace(string(base.s.passwd), "HomeNode install ", "Foreign install ", 1))
	if _, err := engine.provisionGatewayAccount(ctx, backend); !errors.Is(err, ErrConflict) || base.commands != 5 {
		t.Fatal("lost base ownership mutated accounts", err)
	}
}

type gatewayVerificationFailure struct{ fixtureGatewayProvisioner }

func (gatewayVerificationFailure) VerifyGateway(context.Context) (GatewayAccount, error) {
	return GatewayAccount{}, ErrAccounts
}

func TestGatewayVerificationFailureCannotCommitReadiness(t *testing.T) {
	host, journal := roots(t)
	engine := openEngine(t, host, journal)
	defer engine.Close()
	base := newAccountFixture()
	ctx := context.Background()
	if _, err := engine.provisionAccounts(ctx, base); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.prepareGatewayAccount(ctx, base); err != nil {
		t.Fatal(err)
	}
	fixture := fixtureGatewayProvisioner{base}
	if _, err := engine.provisionGatewayAccount(ctx, gatewayVerificationFailure{fixture}); !errors.Is(err, ErrAccounts) {
		t.Fatal("failed verification ignored", err)
	}
	owned, err := engine.loadAccountJournal()
	if err != nil {
		t.Fatal(err)
	}
	pending, err := engine.loadGatewayAccountJournal(owned)
	if err != nil || pending.Ready || pending.Completed != 4 || base.commands != 9 {
		t.Fatal("unverified identity marked ready", pending, base.commands, err)
	}
	if _, err := engine.provisionGatewayAccount(ctx, fixture); err != nil || base.commands != 9 {
		t.Fatal("verification retry replayed mutations", err)
	}
}
