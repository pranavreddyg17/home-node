package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/pranavreddyg17/home-node/internal/install"
)

func parseUpdateInitialization(args []string) (string, error) {
	flags := flag.NewFlagSet("update-trust-initialize", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	directory := flags.String("journal-dir", "/var/lib/homenode-install", "existing private installation journal")
	if err := flags.Parse(args); err != nil {
		return "", err
	}
	if flags.NArg() != 0 || !filepath.IsAbs(*directory) || filepath.Clean(*directory) != *directory {
		return "", install.ErrPlan
	}
	return *directory, nil
}

func initializeUpdateTrust(args []string) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		fatal(errors.New("update trust initialization requires Linux and root"))
	}
	directory, err := parseUpdateInitialization(args)
	if err != nil {
		fatal(err)
	}
	engine, err := install.Open("/", directory)
	if err != nil {
		fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	initializeErr := engine.InitializeUpdateCache(ctx)
	closeErr := engine.Close()
	if err = errors.Join(initializeErr, closeErr, ctx.Err()); err != nil {
		fatal(err)
	}
	if err = json.NewEncoder(os.Stdout).Encode(map[string]bool{"updateTrustInitialized": true, "updatesActivated": false}); err != nil {
		fatal(err)
	}
}
