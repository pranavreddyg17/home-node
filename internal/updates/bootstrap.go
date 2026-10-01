package updates

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/theupdateframework/go-tuf/v2/metadata"
	"github.com/theupdateframework/go-tuf/v2/metadata/trustedmetadata"
)

// BootstrapRoot is supplied through independently trusted release setup. A
// digest copied from the same untrusted download does not establish trust.
// It is immutable installer configuration; the current cache root rotates.
type BootstrapRoot struct {
	Data   []byte
	SHA256 string
}

func (root BootstrapRoot) Validate() (int64, error) {
	if len(root.Data) == 0 || len(root.Data) > 128<<10 || len(root.SHA256) != 64 {
		return 0, errReleasePolicy
	}
	sum := sha256.Sum256(root.Data)
	if hex.EncodeToString(sum[:]) != root.SHA256 {
		return 0, errReleasePolicy
	}
	trusted, err := trustedmetadata.New(root.Data)
	if err != nil {
		return 0, errReleasePolicy
	}
	signed := trusted.Root.Signed
	if signed.Version < 1 || len(signed.Roles) != 4 || len(signed.Keys) > 32 {
		return 0, errReleasePolicy
	}
	assigned := map[string]bool{}
	for _, name := range []string{metadata.ROOT, metadata.TIMESTAMP, metadata.SNAPSHOT, metadata.TARGETS} {
		role := signed.Roles[name]
		if role == nil || role.Threshold < 1 || role.Threshold > len(role.KeyIDs) || name == metadata.ROOT && role.Threshold < 2 {
			return 0, errReleasePolicy
		}
		for _, id := range role.KeyIDs {
			if assigned[id] || signed.Keys[id] == nil {
				return 0, errReleasePolicy
			}
			assigned[id] = true
		}
	}
	return signed.Version, nil
}
