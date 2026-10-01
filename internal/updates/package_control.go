package updates

import (
	"context"
	"os"
)

// ValidateDebianControl joins outer archive structure and bounded control
// inspection on the same already-open package descriptor. It leaves the file
// offset unchanged. Passing this check is not payload validation, evidence
// qualification or permission to install. xz requires the inspection worker.
func ValidateDebianControl(ctx context.Context, file *os.File, release ReleaseMetadata) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	archive, err := InspectDebianArchive(file)
	if err != nil {
		return err
	}
	return ValidateCompressedControl(ctx, archive.Control, archive.ControlCompression, release)
}
