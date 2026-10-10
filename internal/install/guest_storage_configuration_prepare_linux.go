//go:build linux

package install

import (
	"context"
	"errors"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// Caller retains installed-state, storage, account and runtime exclusion. The
// bounded reads are candidates only; both exact protected source scopes must
// qualify and remain open before an immutable transition is recorded.
func (e *Engine) prepareGuestStorageConfigurationIntent(ctx context.Context, installed journal, plan GuestStorageProvisioningPlan, guard func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if e == nil || e.host == nil || guard == nil || os.Geteuid() != 0 {
		return ErrPlan
	}
	if err := guard(ctx); err != nil {
		return err
	}
	policy, err := e.readGuestStorageConfigurationCandidate(ctx, installed, "runtime-policy.json", 0600)
	if err != nil {
		return err
	}
	environment, err := e.readGuestStorageConfigurationCandidate(ctx, installed, "services.env", 0644)
	if err != nil {
		return err
	}
	return e.withGuestStorageConfigurationSource(ctx, "runtime-policy.json", string(policy), func(ctx context.Context, _ *os.File, checkPolicy func() error) error {
		return e.withGuestStorageConfigurationSource(ctx, "services.env", string(environment), func(ctx context.Context, _ *os.File, checkEnvironment func() error) error {
			check := func(ctx context.Context) error {
				if err := guard(ctx); err != nil {
					return err
				}
				for _, verify := range []func() error{checkPolicy, checkEnvironment} {
					if err := verify(); err != nil {
						return err
					}
				}
				return guard(ctx)
			}
			return e.commitGuestStorageConfigurationIntent(ctx, installed, policy, environment, plan, check)
		})
	})
}

func (e *Engine) readGuestStorageConfigurationCandidate(ctx context.Context, installed journal, name string, mode uint32) (data []byte, result error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if name != "runtime-policy.json" && name != "services.env" || name == "runtime-policy.json" && mode != 0600 || name == "services.env" && mode != 0644 {
		return nil, ErrPlan
	}
	var expected string
	for _, item := range installed.Items {
		if item.Path != "etc/homenode/"+name {
			continue
		}
		if expected != "" || item.Directory || item.UID != 0 || item.GID != 0 || item.Mode != mode || len(item.SHA256) != 64 || item.State != "created" && item.State != "existing" {
			return nil, ErrConflict
		}
		expected = item.SHA256
	}
	if expected == "" {
		return nil, ErrConflict
	}
	file, err := e.host.OpenFile("etc/homenode/"+name, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer func() {
		result = errors.Join(result, file.Close())
		if result != nil {
			data = nil
		}
	}()
	var stat unix.Stat_t
	if unix.Fstat(int(file.Fd()), &stat) != nil || stat.Mode != unix.S_IFREG|mode || stat.Uid != 0 || stat.Gid != 0 || stat.Nlink != 1 || stat.Size < 1 || stat.Size > 16384 {
		return nil, ErrConflict
	}
	data, err = io.ReadAll(io.LimitReader(file, 16385))
	if err != nil {
		return nil, err
	}
	if len(data) != int(stat.Size) || digest(data) != expected {
		return nil, ErrConflict
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return data, nil
}
