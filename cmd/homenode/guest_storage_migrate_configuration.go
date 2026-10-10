package main

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/pranavreddyg17/home-node/internal/install"
)

func guestStorageMigrateConfiguration(args []string) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Hour)
	defer cancel()
	if err := runGuestStorageMigrateConfiguration(ctx, args, os.Stdout); err != nil {
		fatal(err)
	}
}

func runGuestStorageMigrateConfiguration(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("guest-storage-migrate-configuration", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	journal := flags.String("journal-dir", "/var/lib/homenode-install", "existing private installer journal")
	key := flags.String("publisher-key", "", "independently verified publisher Ed25519 public key, hex")
	floor := flags.Int64("minimum-catalog-version", 0, "independent minimum accepted catalog version")
	if err := flags.Parse(args); err != nil {
		return err
	}
	public, err := hex.DecodeString(*key)
	if err != nil || len(public) != ed25519.PublicKeySize || *floor < 1 || *journal == "" || flags.NArg() != 0 {
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
	result, err := engine.MigrateGuestStorageConfiguration(ctx, ed25519.PublicKey(public), *floor)
	if err := errors.Join(err, engine.Close()); err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(result)
}
