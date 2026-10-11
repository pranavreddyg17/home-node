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

func gatewayTLSRetirement(args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if err := runGatewayTLSRetirement(ctx, args, os.Stdout); err != nil {
		fatal(err)
	}
}
func runGatewayTLSRetirement(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("gateway-tls-retire", flag.ContinueOnError)
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
	err = engine.RetireGatewayTLS(ctx)
	if err = errors.Join(err, engine.Close()); err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(map[string]any{
		"phase": "gateway-tls-retire", "tlsOwnershipRetired": true, "tlsContentsDeleted": false,
		"activationBlockRetained": true, "servicesDormant": true, "servicesActivated": false,
		"gatewayIdentityRemoved": false, "installationRemoved": false, "packageRemoved": false,
		"applicationDataDeleted": false, "backupDataDeleted": false,
	})
}
