//go:build linux

package backup

import (
	"context"
	"time"
)

// InspectSnapshot checks selected snapshot metadata without extracting payloads.
// Policy must come from the trusted installed release/catalog. This preview
// neither certifies payload/filesystem integrity nor grants recovery authority.
func (r *Repository) InspectSnapshot(ctx context.Context, snapshot string, policy RestorePolicy) (Manifest, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Manifest{}, err
	}
	if r.directory == nil || !repositoryPattern.MatchString(snapshot) {
		return Manifest{}, ErrRepository
	}
	deadline, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	output := &boundedOutput{maximum: MaxManifestBytes}
	if err := r.dump(deadline, snapshot, "manifest.json", output); err != nil {
		return Manifest{}, err
	}
	manifest, err := DecodeManifest(output.data)
	if err != nil || manifest.Validate(policy, time.Now()) != nil {
		return Manifest{}, ErrManifest
	}
	if err := deadline.Err(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}
