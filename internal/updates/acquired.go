package updates

import (
	"encoding/json"
	"os"
)

// AcquiredRelease holds verified bytes for later policy review and installation.
// Its evidence has not yet passed vulnerability or build-identity review.
type AcquiredRelease struct {
	PackageSHA256 string          `json:"-"`
	PackageLength int64           `json:"-"`
	Package       *os.File        `json:"-"`
	Metadata      ReleaseMetadata `json:"-"`
	SBOM          json.RawMessage `json:"-"`
	Provenance    json.RawMessage `json:"-"`
}

func (release *AcquiredRelease) Close() error { return release.Package.Close() }
