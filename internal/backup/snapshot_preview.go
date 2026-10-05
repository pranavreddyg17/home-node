package backup

import (
	"context"
	"time"
)

// SnapshotPreview reports declared compatibility metadata only. No source
// paths, payload/image hashes, credentials or recovered authority are exposed.
// Compatible metadata does not certify payload integrity or app health.
type SnapshotPreview struct {
	SnapshotID     string                `json:"snapshotId"`
	CreatedAt      time.Time             `json:"createdAt"`
	Release        string                `json:"release"`
	CatalogVersion int64                 `json:"catalogVersion"`
	Validation     string                `json:"validation"`
	Files          []SnapshotPreviewFile `json:"files"`
}

type SnapshotPreviewFile struct {
	Workload string `json:"workload"`
	Bytes    int64  `json:"bytes"`
}

func (r *Repository) PreviewSnapshot(ctx context.Context, snapshot string, policy RestorePolicy) (SnapshotPreview, error) {
	manifest, err := r.InspectSnapshot(ctx, snapshot, policy)
	if err != nil {
		return SnapshotPreview{}, err
	}
	if err := ctx.Err(); err != nil {
		return SnapshotPreview{}, err
	}
	return previewSnapshotManifest(snapshot, manifest, policy, time.Now())
}

func previewSnapshotManifest(snapshot string, manifest Manifest, policy RestorePolicy, now time.Time) (SnapshotPreview, error) {
	if !repositoryPattern.MatchString(snapshot) || manifest.Validate(policy, now) != nil {
		return SnapshotPreview{}, ErrManifest
	}
	preview := SnapshotPreview{SnapshotID: snapshot, CreatedAt: manifest.CreatedAt, Release: manifest.Release, CatalogVersion: manifest.CatalogVersion, Validation: "metadata-compatible", Files: make([]SnapshotPreviewFile, 0, len(manifest.Files))}
	for _, file := range manifest.Files {
		preview.Files = append(preview.Files, SnapshotPreviewFile{Workload: file.Workload, Bytes: file.Bytes})
	}
	return preview, nil
}
