package backup

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestSnapshotPreviewContainsOnlyCompatibleDeclaredMetadata(t *testing.T) {
	manifest, policy, _ := manifestFixture()
	snapshot := strings.Repeat("a", 64)
	preview, err := previewSnapshotManifest(snapshot, manifest, policy, time.Now())
	if err != nil || preview.SnapshotID != snapshot || preview.Validation != "metadata-compatible" || len(preview.Files) != 1 || preview.Files[0].Bytes != manifest.Files[0].Bytes {
		t.Fatal(preview, err)
	}
	raw, err := json.Marshal(preview)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{manifest.Files[0].Name, manifest.Files[0].SHA256, "imageSha256", "managementSchema", "platform"} {
		if strings.Contains(string(raw), private) {
			t.Fatal("preview exposes unnecessary manifest details", private)
		}
	}
	policy.MinimumCatalogVersion = manifest.CatalogVersion + 1
	if preview, err := previewSnapshotManifest(snapshot, manifest, policy, time.Now()); err == nil || preview.Files != nil {
		t.Fatal("incompatible metadata returned", preview, err)
	}
	if _, err := previewSnapshotManifest("latest", manifest, policy, time.Now()); err == nil {
		t.Fatal("ambiguous selection admitted")
	}
}
