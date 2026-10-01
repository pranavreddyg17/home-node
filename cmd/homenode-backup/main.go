package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"syscall"

	"github.com/pranavreddyg17/home-node/internal/runtimeclient"
	"github.com/pranavreddyg17/home-node/internal/socketactivation"
)

var errConfiguration = errors.New("invalid trusted backup service configuration")

type options struct {
	controllerUID, controllerGID uint
	socket                       string
	worker                       runtimeclient.BackupWorkerConfig
}

func parseOptions(args []string) (options, error) {
	var option options
	flags := flag.NewFlagSet("homenode-backup", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	flags.UintVar(&option.controllerUID, "controller-uid", 0, "installed controller UID")
	flags.UintVar(&option.controllerGID, "controller-gid", 0, "installed controller GID")
	flags.StringVar(&option.socket, "socket", "/run/homenode-backup/credential.sock", "named activated credential listener")
	flags.StringVar(&option.worker.ManagementSocket, "management-socket", "/run/homenode-backup/apps.sock", "private management socket")
	flags.StringVar(&option.worker.DiskSocket, "disk-socket", "/run/homenode/maintenance-disk.sock", "private root disk socket")
	flags.StringVar(&option.worker.StagingParent, "staging", "/var/lib/homenode-backup/staging", "provisioned private staging parent")
	flags.StringVar(&option.worker.RepositoryTarget.MountPath, "repository-mount", "", "registered external drive mount")
	flags.StringVar(&option.worker.RepositoryTarget.UUID, "repository-uuid", "", "registered external drive UUID")
	flags.StringVar(&option.worker.RepositoryTarget.RepositoryID, "repository-id", "", "registered authenticated repository ID")
	flags.StringVar(&option.worker.Release, "release", "", "verified installed release")
	flags.Int64Var(&option.worker.CatalogVersion, "catalog-version", 0, "verified installed catalog version")
	flags.Int64Var(&option.worker.Policy.MinimumCatalogVersion, "minimum-catalog-version", 0, "verified recovery catalog floor")
	if err := flags.Parse(args); err != nil {
		return option, err
	}
	if flags.NArg() != 0 || option.controllerUID < 100 || option.controllerUID > 999 || option.controllerGID < 100 || option.controllerGID > 999 {
		return option, errConfiguration
	}
	for _, path := range []string{option.socket, option.worker.ManagementSocket, option.worker.DiskSocket, option.worker.StagingParent} {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return option, errConfiguration
		}
	}
	if option.socket == option.worker.ManagementSocket || option.socket == option.worker.DiskSocket || option.worker.ManagementSocket == option.worker.DiskSocket {
		return option, errConfiguration
	}
	if !regexp.MustCompile(`^[0-9][a-zA-Z0-9.+~-]{0,63}$`).MatchString(option.worker.Release) || option.worker.Policy.MinimumCatalogVersion < 1 || option.worker.CatalogVersion < option.worker.Policy.MinimumCatalogVersion {
		return option, errConfiguration
	}
	if err := option.worker.RepositoryTarget.Validate(); err != nil {
		return option, errConfiguration
	}
	return option, nil
}
func run(ctx context.Context, args []string) error {
	option, err := parseOptions(args)
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	uid, gid := os.Geteuid(), os.Getegid()
	if runtime.GOOS != "linux" || uid < 100 || uid > 999 || gid < 100 || gid > 999 || uint(uid) == option.controllerUID || uint(gid) == option.controllerGID {
		return errConfiguration
	}
	groups, err := os.Getgroups()
	if err != nil {
		return errConfiguration
	}
	for _, group := range groups {
		if group != gid {
			return errConfiguration
		}
	}
	// Consume activation before opening application files. The root-created
	// management listener is selected explicitly by ControllerListenerUID zero.
	listener, err := socketactivation.TakePrivatePacketListener("homenode-backup-credential", option.socket, uint32(option.controllerGID))
	if err != nil {
		return err
	}
	return runtimeclient.ServeRegisteredBackupWorker(ctx, listener, uint32(option.controllerUID), option.worker)
}
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "backup service stopped:", err)
		os.Exit(1)
	}
}
