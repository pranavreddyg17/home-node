package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"github.com/pranavreddyg17/home-node/internal/install"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
	"io"
	"os"
	"runtime"
	"time"
)

func guestStorage(args []string, mode string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := runGuestStorage(ctx, args, os.Stdout, mode); err != nil {
		fatal(err)
	}
}

func runGuestStorage(ctx context.Context, args []string, out io.Writer, mode string) error {
	if mode != "plan" && mode != "prepare" && mode != "check" {
		return install.ErrPlan
	}
	flags := flag.NewFlagSet("guest-storage-"+mode, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	journal := flags.String("journal-dir", "/var/lib/homenode-install", "existing private installer journal")
	first := flags.Uint64("first-uid", 0, "first proposed reserved guest UID")
	last := flags.Uint64("last-uid", 0, "last proposed reserved guest UID")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *journal == "" {
		return install.ErrPlan
	}
	if mode == "check" {
		if *first != 0 || *last != 0 {
			return install.ErrPlan
		}
	} else {
		if *first < 65536 || *last < *first || *last > 1<<31-1 || *last-*first >= 65536 {
			return install.ErrPlan
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return install.ErrAccounts
	}
	plan, err := loadGuestStorageAction(ctx, *journal, supervisor.GuestUIDPool{First: uint32(*first), Last: uint32(*last)}, mode)
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(map[string]any{"plan": plan, "intentCommitted": mode == "prepare", "intentValid": mode == "check", "policyPublished": false, "servicesActivated": false, "activationQualified": false})
}

func loadGuestStorageAction(ctx context.Context, journal string, pool supervisor.GuestUIDPool, mode string) (plan install.GuestStorageProvisioningPlan, result error) {
	engine, err := install.Open("/", journal)
	if err != nil {
		return plan, err
	}
	defer func() {
		result = errors.Join(result, engine.Close())
		if result != nil {
			plan = install.GuestStorageProvisioningPlan{}
		}
	}()
	switch mode {
	case "plan":
		return engine.PlanInstalledGuestStorageProvisioning(ctx, pool)
	case "prepare":
		return engine.PrepareGuestStorageProvisioning(ctx, pool)
	case "check":
		return engine.CheckGuestStorageProvisioningIntent(ctx)
	default:
		return plan, install.ErrPlan
	}
}
