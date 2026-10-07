package main

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"os"
	"runtime"
	"time"

	"github.com/pranavreddyg17/home-node/internal/install"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

func guestUIDPlan(args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := runGuestUIDPlan(ctx, args, os.Stdout); err != nil {
		fatal(err)
	}
}

// This command only observes eligibility and emits pending provisioning gates.
// Explicit service UIDs are proposal inputs, not proof of installed identities.
func runGuestUIDPlan(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("guest-uid-plan", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	journal := flags.String("journal-dir", "", "derive identities from an existing installer journal")
	owner := flags.String("owner-id", "", "installer ownership ID for this proposal")
	first := flags.Uint64("first-uid", 0, "first proposed reserved guest UID")
	last := flags.Uint64("last-uid", 0, "last proposed reserved guest UID")
	controller := flags.Uint64("controller-uid", 0, "controller service UID")
	transfer := flags.Uint64("transfer-uid", 0, "transfer service UID")
	backup := flags.Uint64("backup-uid", 0, "backup service UID, when configured")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *first > 1<<31-1 || *last > 1<<31-1 || *controller > 1<<31-1 || *transfer > 1<<31-1 || *backup > 1<<31-1 {
		return install.ErrPlan
	}
	if *journal != "" && (*owner != "" || *controller != 0 || *transfer != 0 || *backup != 0) {
		return install.ErrPlan
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return install.ErrConflict
	}
	identities := []uint32{uint32(*controller), uint32(*transfer)}
	if *backup != 0 {
		identities = append(identities, uint32(*backup))
	}
	pool := supervisor.GuestUIDPool{First: uint32(*first), Last: uint32(*last)}
	var plan install.GuestUIDProvisioningPlan
	var err error
	if *journal != "" {
		engine, openErr := install.Open("/", *journal)
		if openErr != nil {
			return openErr
		}
		defer engine.Close()
		plan, err = engine.PlanInstalledGuestUIDProvisioning(ctx, pool)
	} else {
		plan, err = install.PlanGuestUIDProvisioning(ctx, *owner, pool, identities)
	}
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(map[string]any{"plan": plan, "policyPublished": false, "servicesActivated": false})
}
