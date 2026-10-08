package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/pranavreddyg17/home-node/internal/install"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

func guestAllocationPrepare(args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := runGuestAllocationPrepare(ctx, args, os.Stdout); err != nil {
		fatal(err)
	}
}

func runGuestAllocationPrepare(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("guest-allocation-prepare", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	journal := flags.String("journal-dir", "/var/lib/homenode-install", "existing private installation journal")
	names := []string{"first-uid", "last-uid", "uid-min", "uid-max", "sys-uid-min", "sys-uid-max", "sub-uid-min", "sub-uid-max"}
	values := make([]*string, len(names))
	for i, name := range names {
		values[i] = flags.String(name, "", "explicit decimal UID boundary")
	}
	seen := map[string]bool{}
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			continue
		}
		name, _, _ := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		if flags.Lookup(name) == nil {
			continue
		}
		if seen[name] {
			return install.ErrPlan
		}
		seen[name] = true
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *journal == "" {
		return install.ErrPlan
	}
	parsed := make([]uint32, len(values))
	for i, value := range values {
		number, err := strconv.ParseUint(*value, 10, 32)
		if err != nil || number == 0 || strconv.FormatUint(number, 10) != *value {
			return install.ErrPlan
		}
		parsed[i] = uint32(number)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	pool := supervisor.GuestUIDPool{First: parsed[0], Last: parsed[1]}
	selection := install.GuestUIDAllocatorRanges{NormalFirst: parsed[2], NormalLast: parsed[3], SystemFirst: parsed[4], SystemLast: parsed[5], SubordinateFirst: parsed[6], SubordinateLast: parsed[7]}
	configuration := fmt.Sprintf("UID_MIN %d\nUID_MAX %d\nSYS_UID_MIN %d\nSYS_UID_MAX %d\nSUB_UID_MIN %d\nSUB_UID_MAX %d\n", parsed[2], parsed[3], parsed[4], parsed[5], parsed[6], parsed[7])
	if err := supervisor.ValidateGuestUIDAutomaticAllocationConfiguration(ctx, pool, []byte(configuration)); err != nil {
		return err
	}
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return install.ErrAccounts
	}
	engine, err := install.Open("/", *journal)
	if err != nil {
		return err
	}
	preview, err := engine.PrepareGuestUIDAllocationConfiguration(ctx, pool, selection)
	if err = errors.Join(err, engine.Close()); err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(preview)
}
