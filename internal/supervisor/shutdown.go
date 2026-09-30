package supervisor

import (
	"context"
	"time"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
)

// Shutdown requests cooperative ACPI poweroff and observes domain exit. It never
// falls back to destroy. Domain exit alone does not attest a clean guest unmount;
// backup orchestration must establish that separately before copying disks.
// This method is intentionally not exposed as an unreviewed supervisor action.
func (b LinuxBackend) Shutdown(ctx context.Context, id string) error {
	deadline, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	return awaitShutdown(deadline, id, b.Running, func(ctx context.Context, id string) error {
		_, err := command(ctx, "", "/usr/bin/virsh", "--connect", "qemu:///system", "shutdown", "homenode-"+id, "--mode", "acpi")
		return err
	}, 250*time.Millisecond)
}

func awaitShutdown(ctx context.Context, id string, running func(context.Context, string) (bool, error), request func(context.Context, string) error, interval time.Duration) error {
	if !guestproto.ValidID(id) || interval <= 0 {
		return ErrPolicy
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	active, err := running(ctx, id)
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if !active {
		return nil
	}
	if err = request(ctx, id); err != nil {
		return err
	}
	for {
		if err = ctx.Err(); err != nil {
			return err
		}
		active, err = running(ctx, id)
		if err != nil {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		if !active {
			return nil
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func shutdownDeadlineKey(id string) string { return "runtime.shutdown-deadline." + id }
