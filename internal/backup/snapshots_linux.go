//go:build linux

package backup

import (
	"context"
	"os"
	"time"
)

// Snapshots lists bounded candidates from the retained authenticated repository.
// Selection still requires manifest, payload, authority and filesystem checks.
func (r *Repository) Snapshots(ctx context.Context) ([]SnapshotReference, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.directory == nil {
		return nil, ErrRepository
	}
	deadline, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	output := &boundedOutput{maximum: maxSnapshotInventoryBytes}
	args := []string{"--repo", "/proc/self/fd/3", "--password-file", "/proc/self/fd/4", "--no-cache", "snapshots", "--json", "--tag", "homenode-v1"}
	if err := resticProcess(deadline, args, []*os.File{r.directory, r.secret}, output); err != nil {
		return nil, err
	}
	if err := deadline.Err(); err != nil {
		return nil, err
	}
	return parseSnapshotInventory(output.data, time.Now())
}
