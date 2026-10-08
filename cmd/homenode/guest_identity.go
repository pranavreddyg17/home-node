package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"runtime"
	"time"

	"github.com/pranavreddyg17/home-node/internal/install"
)

func guestIdentityPrepare(args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := runGuestIdentityPrepare(ctx, args, os.Stdout); err != nil {
		fatal(err)
	}
}

func runGuestIdentityPrepare(ctx context.Context, args []string, out io.Writer) error {
	return runGuestIdentityConfiguration(ctx, args, out, false)
}

func guestIdentityApply(args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := runGuestIdentityApply(ctx, args, os.Stdout); err != nil {
		fatal(err)
	}
}

func runGuestIdentityApply(ctx context.Context, args []string, out io.Writer) error {
	return runGuestIdentityConfiguration(ctx, args, out, true)
}

func runGuestIdentityConfiguration(ctx context.Context, args []string, out io.Writer, apply bool) error {
	name := "guest-identity-prepare"
	if apply {
		name = "guest-identity-apply"
	}
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	journal := flags.String("journal-dir", "/var/lib/homenode-install", "existing private installation journal")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *journal == "" {
		return install.ErrPlan
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return install.ErrAccounts
	}
	engine, err := install.Open("/", *journal)
	if err != nil {
		return err
	}
	var preview install.GuestIdentityConfigurationPreview
	if apply {
		preview, err = engine.ApplyGuestIdentityConfiguration(ctx)
	} else {
		preview, err = engine.PrepareGuestIdentityConfiguration(ctx)
	}
	if err = errors.Join(err, engine.Close()); err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(preview)
}
