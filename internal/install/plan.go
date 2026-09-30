// Package install owns the resumable configuration phase of host provisioning.
// It never edits general network/OS configuration or removes owner data.
package install

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path"
)

var ErrConflict = errors.New("installation ownership conflict; preserve the existing path and inspect the journal")
var ErrPlan = errors.New("invalid installation plan")

const maxFileBytes = 128 << 10

type Item struct {
	Path      string `json:"path"`
	Directory bool   `json:"directory"`
	Mode      uint32 `json:"mode"`
	UID       int    `json:"uid"`
	GID       int    `json:"gid"`
	Data      []byte `json:"-"`
}
type Plan struct{ Items []Item }
type record struct {
	Path      string `json:"path"`
	Directory bool   `json:"directory"`
	Mode      uint32 `json:"mode"`
	UID       int    `json:"uid"`
	GID       int    `json:"gid"`
	SHA256    string `json:"sha256,omitempty"`
	State     string `json:"state"`
}
type journal struct {
	Version int      `json:"version"`
	ID      string   `json:"id"`
	Digest  string   `json:"digest"`
	Phase   string   `json:"phase"`
	Items   []record `json:"items"`
}

var directories = map[string]bool{
	"etc/homenode": true, "etc/homenode/tls": true,
	"var/lib/homenode": true, "var/lib/homenode/control": true,
	"var/lib/homenode/supervisor": true, "var/lib/homenode/catalog": true,
	"var/lib/homenode/images": true, "var/lib/homenode/volumes": true,
}
var files = map[string]bool{
	"etc/homenode/services.env": true, "etc/homenode/runtime-policy.json": true,
	"etc/homenode/catalog.pub": true, "etc/homenode/catalog-floor": true, "var/lib/homenode/catalog/catalog.json": true,
	"etc/systemd/system/homenode-control.service":    true,
	"etc/systemd/system/homenode-supervisor.service": true,
	"etc/systemd/system/homenode-transfer.service":   true,
	"etc/systemd/system/homenode.slice":              true,
}

func digest(data []byte) string { value := sha256.Sum256(data); return hex.EncodeToString(value[:]) }
func validRecord(r record, owner int) bool {
	if path.Clean(r.Path) != r.Path || r.UID < 0 || r.GID < 0 || r.UID > 1<<31-1 || r.GID > 1<<31-1 || r.Mode > 0777 || r.Mode&0022 != 0 {
		return false
	}
	if r.Directory {
		return directories[r.Path] && r.SHA256 == "" && r.Mode&0700 == 0700 && (r.UID == owner || r.Path == "var/lib/homenode/control")
	}
	if !files[r.Path] || r.UID != owner || r.Mode&0111 != 0 || r.Mode&0400 == 0 || len(r.SHA256) != 64 {
		return false
	}
	_, err := hex.DecodeString(r.SHA256)
	return err == nil
}
func planRecords(plan Plan, owner int) ([]record, string, error) {
	if len(plan.Items) == 0 || len(plan.Items) > 32 {
		return nil, "", ErrPlan
	}
	records := make([]record, 0, len(plan.Items))
	seen := map[string]bool{}
	for _, item := range plan.Items {
		r := record{Path: item.Path, Directory: item.Directory, Mode: item.Mode, UID: item.UID, GID: item.GID, State: "pending"}
		if !item.Directory {
			r.SHA256 = digest(item.Data)
		}
		if !validRecord(r, owner) || seen[r.Path] || len(item.Data) > maxFileBytes || (item.Directory && len(item.Data) != 0) {
			return nil, "", ErrPlan
		}
		seen[r.Path] = true
		records = append(records, r)
	}

	for index, r := range records {
		for parent := path.Dir(r.Path); parent != "."; parent = path.Dir(parent) {
			for other, p := range records {
				if p.Path == parent && (other >= index || !p.Directory) {
					return nil, "", ErrPlan
				}
			}
		}
	}
	encoded, _ := json.Marshal(records)
	return records, digest(encoded), nil
}
func mode(r record) os.FileMode { return os.FileMode(r.Mode) }
