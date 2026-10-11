package install

import (
	"bytes"
	"encoding/json"
	"reflect"
)

type TLSRetirementInode struct {
	Path   string `json:"path"`
	Device uint64 `json:"device"`
	Inode  uint64 `json:"inode"`
	Size   int64  `json:"size"`
}

// The immutable source identities accompany the content hashes so a restart
// cannot adopt a replacement inode merely because it has the same contents.
type gatewayTLSRetirementIntent struct {
	Version   int                      `json:"version"`
	Completed int                      `json:"completed,omitempty"`
	Ready     bool                     `json:"ready,omitempty"`
	Plan      GatewayTLSRetirementPlan `json:"plan"`
	Sources   []TLSRetirementInode     `json:"sources"`
}

const gatewayTLSRetirementJournalLimit = 16 << 10

// Decode requires an independently reconstructed source plan. Canonical bytes
// reject duplicate fields, unknown fields, altered plans and alternate encodings.
func decodeGatewayTLSRetirementIntent(data []byte, source GatewayTLSRetirementPlan) (gatewayTLSRetirementIntent, error) {
	var intent gatewayTLSRetirementIntent
	if len(data) == 0 || len(data) > gatewayTLSRetirementJournalLimit || json.Unmarshal(data, &intent) != nil || intent.Version != 1 || intent.Completed < 0 || intent.Completed > 5 || (intent.Ready && intent.Completed != 5) || !reflect.DeepEqual(intent.Plan, source) || len(intent.Sources) != 3 {
		return intent, ErrConflict
	}

	sourceState := []ConfigurationRemovalFile{source.Key, source.Certificate, {Path: "etc/homenode/tls", UID: 0, GID: source.Identity.GID, Mode: 0750}}
	if _, err := gatewayTLSRetirementState(source, 0, false, sourceState); err != nil {
		return intent, ErrConflict
	}
	expectedPaths := []string{source.Key.Path, source.Certificate.Path, "etc/homenode/tls"}
	for i, item := range intent.Sources {
		if item.Path != expectedPaths[i] || item.Inode == 0 || item.Size < 0 || (i < 2 && (item.Size == 0 || item.Size > 16<<10)) {
			return intent, ErrConflict
		}
		for j := 0; j < i; j++ {
			if intent.Sources[j].Device == item.Device && intent.Sources[j].Inode == item.Inode {
				return intent, ErrConflict
			}
		}
	}
	canonical, err := json.Marshal(intent)
	if err != nil || !bytes.Equal(data, canonical) {
		return intent, ErrConflict
	}
	return intent, nil
}
