package updates

import (
	"context"
	"os"
)

// ValidateDebianContent inspects structure, release-bound control and complete
// payload inventory through one package descriptor without extraction. It
// preserves descriptor offset. Passing does not prove signed authenticity,
// build identity, vulnerability qualification or approval to install. Use in
// the constrained inspection worker; xz support remains pending there.
func ValidateDebianContent(ctx context.Context, file *os.File, release ReleaseMetadata) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	archive, err := InspectDebianArchive(file)
	if err != nil {
		return err
	}
	if err = ValidateCompressedControl(ctx, archive.Control, archive.ControlCompression, release); err != nil {
		return err
	}
	return ValidateCompressedPayload(ctx, archive.Data, archive.DataCompression)
}
