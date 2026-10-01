package backup

import (
	"errors"
	"os"
)

// removeOwnedStaging removes only the inodes created by this operation. Staging
// must remain exclusively owned through inspection and unlink; this check does
// not provide a race-proof unlink against an independent privileged writer.
func removeOwnedStaging(root *os.Root, created map[string]os.FileInfo) error {
	var result error
	for name, original := range created {
		current, err := root.Lstat(name)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			result = errors.Join(result, err)
			continue
		}
		if !os.SameFile(original, current) {
			result = errors.Join(result, ErrManifest)
			continue
		}
		result = errors.Join(result, root.Remove(name))
	}
	return result
}
