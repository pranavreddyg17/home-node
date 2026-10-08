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

func recoveryQuiesce(args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := runRecoveryQuiesce(ctx, args, os.Stdout); err != nil {
		fatal(err)
	}
}

func runRecoveryQuiesce(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("recovery-quiesce", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	journal := flags.String("journal-dir", "/var/lib/homenode-install", "existing private recovery installation journal")
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
	err = engine.QuiesceRecovery(ctx)
	if err = errors.Join(err, engine.Close()); err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(map[string]bool{"servicesDormant": true, "guestSubtreeEmpty": true, "publicationAuthorized": false, "storageMigrated": false, "servicesActivated": false, "activationReleased": false})
}
