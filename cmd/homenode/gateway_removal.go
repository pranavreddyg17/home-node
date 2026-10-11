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

func gatewayRemoval(args []string, mode string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if err := runGatewayRemoval(ctx, args, mode, os.Stdout); err != nil {
		fatal(err)
	}
}

func runGatewayRemoval(ctx context.Context, args []string, mode string, out io.Writer) error {
	if mode != "plan" && mode != "prepare" && mode != "apply" {
		return install.ErrPlan
	}
	flags := flag.NewFlagSet("gateway-accounts-removal-"+mode, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	directory := flags.String("journal-dir", "/var/lib/homenode-install", "existing private installation journal")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *directory == "" {
		return install.ErrPlan
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return install.ErrAccounts
	}
	engine, err := install.Open("/", *directory)
	if err != nil {
		return err
	}
	var plan install.GatewayAccountRemovalPlan
	switch mode {
	case "plan":
		plan, err = engine.PlanGatewayAccountRemoval(ctx)
	case "prepare":
		plan, err = engine.PrepareGatewayAccountRemoval(ctx)
	case "apply":
		err = engine.RemoveGatewayAccount(ctx)
	}
	if err = errors.Join(err, engine.Close()); err != nil {
		return err
	}
	result := map[string]any{"phase": mode, "gatewayAccountRemoved": mode == "apply", "removalIntentCommitted": mode != "plan", "installationRemoved": false, "servicesActivated": false, "applicationDataDeleted": false, "backupDataDeleted": false}
	if mode == "apply" {
		result["activationBlockRetained"] = true
		result["servicesDormant"] = true
	} else {
		result["plan"] = plan
	}
	return json.NewEncoder(out).Encode(result)
}
