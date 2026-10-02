//go:build linux

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"regexp"
	"syscall"
	"time"

	"github.com/pranavreddyg17/home-node/internal/updates"
	"golang.org/x/sys/unix"
)

var workerOperation = regexp.MustCompile(updates.InspectionOperationPattern)
var workerRelease = regexp.MustCompile(`^[0-9][a-zA-Z0-9.+~-]{0,63}$`)

func validWorkerArguments(operation, release string) bool {
	return len(operation) >= 20 && len(operation) <= 64 && len(release) >= 1 && len(release) <= 64 && workerOperation.MatchString(operation) && workerRelease.MatchString(release)
}

func run(ctx context.Context, args []string, output io.Writer) error {
	flags := flag.NewFlagSet("homenode-inspect", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	operation := flags.String("operation", "", "maintenance operation identity supplied by trusted launcher")
	release := flags.String("release", "", "expected signed release identity supplied by maintenance launcher")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || !validWorkerArguments(*operation, *release) {
		return errors.New("expected release is required")
	}
	if os.Geteuid() == 0 || os.Getegid() == 0 || os.Getuid() != os.Geteuid() || os.Getgid() != os.Getegid() {
		return errors.New("inspection requires an unprivileged worker identity")
	}
	ruid, euid, suid := unix.Getresuid()
	rgid, egid, sgid := unix.Getresgid()
	if ruid != euid || suid != euid || rgid != egid || sgid != egid {
		return errors.New("inspection refuses saved identity authority")
	}
	groups, err := os.Getgroups()
	if err != nil {
		return err
	}
	for _, group := range groups {
		if group != os.Getegid() {
			return errors.New("inspection refuses supplementary authority")
		}
	}
	privileges, err := unix.PrctlRetInt(unix.PR_GET_NO_NEW_PRIVS, 0, 0, 0, 0)
	if err != nil || privileges != 1 {
		return errors.New("inspection requires NoNewPrivileges")
	}
	status, err := os.Open("/proc/self/status")
	if err != nil {
		return err
	}
	data, readErr := io.ReadAll(io.LimitReader(status, 32769))
	if err = errors.Join(readErr, status.Close()); err != nil {
		return err
	}
	if len(data) > 32768 || !emptyInspectionCapabilities(data) {
		return errors.New("inspection requires empty Linux capability sets")
	}
	file := os.NewFile(3, "verified-package")
	if file == nil {
		return errors.New("inherited package descriptor missing")
	}
	defer file.Close()
	if !readOnlyInspectionDescriptor(file.Fd()) {
		return errors.New("inherited package descriptor must grant read-only access")
	}
	info, err := file.Stat()
	if err != nil {
		return err
	}
	native, ok := info.Sys().(*syscall.Stat_t)
	if !ok || native.Uid != 0 || native.Nlink != 1 || !info.Mode().IsRegular() || info.Mode().Perm() != 0400 {
		return errors.New("inherited package must be private root-owned verified bytes")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	digest, length, err := updates.PackageIdentity(ctx, file)
	if err != nil {
		return err
	}
	if err = updates.ValidateDistributionPackage(ctx, file, updates.ReleaseMetadata{Release: *release, Platform: "ubuntu-24.04-amd64"}); err != nil {
		return err
	}
	finalDigest, finalLength, err := updates.PackageIdentity(ctx, file)
	if err != nil || finalDigest != digest || finalLength != length {
		return errors.Join(errors.New("package identity changed during inspection"), err)
	}
	return json.NewEncoder(output).Encode(updates.InspectionResult{OperationID: *operation, Schema: 1, Release: *release, PackageSHA256: digest, PackageLength: length, ContentValid: true})
}

func readOnlyInspectionDescriptor(fd uintptr) bool {
	flags, err := unix.FcntlInt(fd, unix.F_GETFL, 0)
	return err == nil && flags&unix.O_ACCMODE == unix.O_RDONLY && flags&unix.O_PATH == 0
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
