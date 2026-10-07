package main

import (
	"context"
	"encoding/json"
	"flag"
	"github.com/pranavreddyg17/home-node/internal/install"
	"os"
	"runtime"
	"time"
)

func guestUIDCheck(args []string) {
	flags := flag.NewFlagSet("guest-uid-check", flag.ExitOnError)
	journal := flags.String("journal-dir", "/var/lib/homenode-install", "existing private installer journal")
	_ = flags.Parse(args)
	if flags.NArg() != 0 {
		fatal(install.ErrPlan)
	}
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		fatal(install.ErrAccounts)
	}
	engine, err := install.Open("/", *journal)
	if err != nil {
		fatal(err)
	}
	defer engine.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	plan, err := engine.CheckGuestUIDProvisioningIntent(ctx)
	if err != nil {
		fatal(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(map[string]any{"plan": plan, "intentValid": true, "activationQualified": false}); err != nil {
		fatal(err)
	}
}
