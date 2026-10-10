package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/pranavreddyg17/home-node/internal/install"
)

func gatewayAccounts(args []string, mode string) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		fatal(fmt.Errorf("gateway account administration requires Linux and root"))
	}
	flags := flag.NewFlagSet("gateway-accounts-"+mode, flag.ExitOnError)
	directory := flags.String("journal-dir", "/var/lib/homenode-install", "existing private installer journal")
	_ = flags.Parse(args)
	if flags.NArg() != 0 {
		fatal(fmt.Errorf("unexpected positional arguments"))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if mode == "check" {
		identity, err := install.InspectGatewayAccount(ctx)
		if err != nil {
			fatal(err)
		}
		if err = json.NewEncoder(os.Stdout).Encode(map[string]any{"gatewayAccountValid": true, "identity": identity, "servicesActivated": false}); err != nil {
			fatal(err)
		}
		return
	}
	// The account phase must already own this journal. Opening a supplied missing
	// directory does not create a new journal or adopt preexisting service users.
	engine, err := install.Open("/", *directory)
	if err != nil {
		fatal(err)
	}
	defer engine.Close()
	if mode == "prepare" {
		plan, err := engine.PrepareGatewayAccount(ctx)
		if err != nil {
			fatal(err)
		}
		if err = json.NewEncoder(os.Stdout).Encode(map[string]any{"gatewayAccountPrepared": true, "plan": plan, "servicesActivated": false}); err != nil {
			fatal(err)
		}
		return
	}
	if mode != "provision" {
		fatal(fmt.Errorf("unknown gateway account phase"))
	}
	identity, err := engine.ProvisionGatewayAccount(ctx)
	if err != nil {
		fatal(err)
	}
	if err = json.NewEncoder(os.Stdout).Encode(map[string]any{"gatewayAccountProvisioned": true, "identity": identity, "servicesActivated": false}); err != nil {
		fatal(err)
	}
}
