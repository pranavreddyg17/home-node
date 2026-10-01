package updates

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path"

	"github.com/theupdateframework/go-tuf/v2/metadata"
)

// downloadReleaseEvidence verifies the bytes of an already TUF-authorized
// evidence target. Valid JSON and target integrity do not establish SBOM
// completeness, vulnerability status or an authorized provenance build identity.
func downloadReleaseEvidence(fetcher *metadataFetcher, target *metadata.TargetFiles, consistent bool) (json.RawMessage, error) {
	if target == nil || !evidenceName.MatchString(target.Path) || path.Clean(target.Path) != target.Path || target.Length < 1 || target.Length > 8<<20 || len(target.Hashes["sha256"]) != sha256.Size {
		return nil, errReleasePolicy
	}
	remote := target.Path
	if consistent {
		remote = path.Join(path.Dir(remote), hex.EncodeToString(target.Hashes["sha256"])+"."+path.Base(remote))
	}
	data, err := fetcher.DownloadFile(fetcher.base.String()+remote, target.Length, 0)
	if err != nil {
		return nil, err
	}
	if target.VerifyLengthHashes(data) != nil || !json.Valid(data) {
		return nil, errReleasePolicy
	}
	return json.RawMessage(data), nil
}
