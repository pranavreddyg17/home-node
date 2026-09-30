package main

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"runtime"
	"time"

	"github.com/pranavreddyg17/home-node/internal/install"
)

func installCheck(args []string) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		fatal(install.ErrConflict)
	}
	flags := flag.NewFlagSet("install-check", flag.ExitOnError)
	directory := flags.String("journal-dir", "/var/lib/homenode-install", "existing private installation journal")
	_ = flags.Parse(args)
	if flags.NArg() != 0 {
		fatal(install.ErrPlan)
	}
	engine, err := install.Open("/", *directory)
	if err != nil {
		fatal(err)
	}
	defer engine.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	result, err := engine.CheckInstallation(ctx)
	if err != nil {
		fatal(err)
	}
	if err = json.NewEncoder(os.Stdout).Encode(result); err != nil {
		fatal(err)
	}
}
