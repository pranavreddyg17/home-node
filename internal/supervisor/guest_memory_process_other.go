//go:build !linux

package supervisor

import "context"

func observeGuestMemoryProcess(context.Context, int, string, int64) error {
	return ErrPolicy
}
