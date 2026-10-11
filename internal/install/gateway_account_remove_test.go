package install

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

type fixtureGatewayRemover struct {
	fixtureGatewayProvisioner
	autoPrivate bool
}

func (b fixtureGatewayRemover) Execute(_ context.Context, c AccountCommand) error {
	b.commands++
	filter := func(data []byte, fields int, drop string) []byte {
		rows, _ := accountLines(data, fields)
		var out strings.Builder
		for _, row := range rows {
			if row[0] == drop {
				continue
			}
			if fields == 4 && row[0] == "homenode-proxy" {
				if c.Program == "/usr/bin/gpasswd" {
					row[3] = "homenode-gateway"
				} else if c.Program == "/usr/sbin/userdel" {
					row[3] = ""
				}
			}
			out.WriteString(strings.Join(row, ":") + "\n")
		}
		return []byte(out.String())
	}
	switch c.Program {
	case "/usr/bin/gpasswd":
		b.s.groups = filter(b.s.groups, 4, "")
	case "/usr/sbin/userdel":
		b.s.passwd = filter(b.s.passwd, 7, "homenode-gateway")
		b.s.shadow = filter(b.s.shadow, 9, "homenode-gateway")
		drop := ""
		if b.autoPrivate {
			drop = "homenode-gateway"
		}
		b.s.groups = filter(b.s.groups, 4, drop)
	case "/usr/sbin/groupdel":
		b.s.groups = filter(b.s.groups, 4, c.Arguments[0])
	default:
		return ErrPlan
	}
	return nil
}

func TestGatewayRemovalResumesLostAcknowledgementsUnderGuard(t *testing.T) {
	for _, auto := range []bool{false, true} {
		for interruption := 10; interruption <= 13; interruption++ {
			if auto && interruption == 13 {
				continue
			}
			t.Run(fmt.Sprintf("%t-%d", auto, interruption), func(t *testing.T) {
				host, journal := roots(t)
				e := openEngine(t, host, journal)
				ctx := context.Background()
				base := newAccountFixture()
				if _, err := e.provisionAccounts(ctx, base); err != nil {
					t.Fatal(err)
				}
				if _, err := e.prepareGatewayAccount(ctx, base); err != nil {
					t.Fatal(err)
				}
				provision := fixtureGatewayProvisioner{base}
				if _, err := e.provisionGatewayAccount(ctx, provision); err != nil {
					t.Fatal(err)
				}
				if _, err := e.prepareGatewayAccountRemoval(ctx, provision); err != nil {
					t.Fatal(err)
				}
				backend := fixtureGatewayRemover{provision, auto}
				lost := errors.New("lost removal acknowledgement")
				e.checkpoint = func(stage, name string) error {
					if stage == "gateway-account-removed" && base.commands == interruption {
						return lost
					}
					return nil
				}
				guard := func(context.Context) error { return nil }
				if err := e.removeGatewayAccountLocked(ctx, backend, guard); !errors.Is(err, lost) {
					t.Fatal(err)
				}
				e.Close()
				e = openEngine(t, host, journal)
				defer e.Close()
				if err := e.removeGatewayAccountLocked(ctx, backend, guard); err != nil {
					t.Fatal(err)
				}
				expected := 13
				if auto {
					expected = 12
				}
				if base.commands != expected {
					t.Fatal("replayed removal", base.commands)
				}
				if err := e.removeGatewayAccountLocked(ctx, backend, guard); err != nil || base.commands != expected {
					t.Fatal("ready replay", err, base.commands)
				}
				owned, _ := e.loadAccountJournal()
				intent, err := e.loadGatewayRemovalIntent(owned)
				if err != nil || !intent.Ready || intent.Completed != 4 {
					t.Fatal(intent, err)
				}
			})
		}
	}
}

func TestGatewayRemovalGuardFailureNeverMutates(t *testing.T) {
	host, journal := roots(t)
	e := openEngine(t, host, journal)
	defer e.Close()
	base := newAccountFixture()
	backend := fixtureGatewayRemover{fixtureGatewayProvisioner{base}, false}
	if err := e.removeGatewayAccountLocked(context.Background(), backend, nil); !errors.Is(err, ErrPlan) || base.commands != 0 {
		t.Fatal(err)
	}
	// An execution guard must be checked even when the intent is ready or a command
	// acknowledgement can be reconciled without running another command.
	ctx := context.Background()
	if _, err := e.provisionAccounts(ctx, base); err != nil {
		t.Fatal(err)
	}
	if _, err := e.prepareGatewayAccount(ctx, base); err != nil {
		t.Fatal(err)
	}
	if _, err := e.provisionGatewayAccount(ctx, backend.fixtureGatewayProvisioner); err != nil {
		t.Fatal(err)
	}
	if _, err := e.prepareGatewayAccountRemoval(ctx, backend.fixtureGatewayProvisioner); err != nil {
		t.Fatal(err)
	}
	denied := errors.New("activation exclusion lost")
	if err := e.removeGatewayAccountLocked(ctx, backend, func(context.Context) error { return denied }); !errors.Is(err, denied) || base.commands != 9 {
		t.Fatal(err, base.commands)
	}
}
