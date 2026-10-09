package install

import (
	"context"
	"encoding/json"
	"math"
	"strings"
)

type guestStorageImageIdentity struct {
	OwnerID   string `json:"ownerId"`
	SHA256    string `json:"sha256"`
	Bytes     int64  `json:"bytes"`
	SourceGID uint32 `json:"sourceGid"`
	GuestGID  uint32 `json:"guestGid"`
	Device    uint64 `json:"device"`
	Inode     uint64 `json:"inode"`
}

type guestStorageImagesIntent struct {
	Version int                          `json:"version"`
	Plan    GuestStorageProvisioningPlan `json:"plan"`
	Images  []guestStorageImageIdentity  `json:"images"`
}

// Caller must retain qualified catalog, image descriptors, pathnames and
// migration exclusion. This records pre-mutation provenance, never authorizes
// adopting an unrecorded image that already has the destination ownership.
func (e *Engine) commitGuestStorageImagesIntent(ctx context.Context, plan GuestStorageProvisioningPlan, images []guestStorageImageIdentity) error {
	qualified, err := canonicalGuestStoragePlan(ctx, plan)
	if err != nil {
		return err
	}
	if len(images) == 0 {
		return ErrPlan
	}
	seen := make(map[[2]uint64]bool)
	previous := ""
	for _, image := range images {
		key := [2]uint64{image.Device, image.Inode}
		if image.OwnerID != qualified.Identity.OwnerID || image.GuestGID != qualified.GuestGID || image.SourceGID == 0 || image.SourceGID > math.MaxInt32 || image.Bytes < 1 || image.Bytes > 64<<30 || len(image.SHA256) != 64 || strings.Trim(image.SHA256, "0123456789abcdef") != "" || image.SHA256 <= previous || image.Device > math.MaxInt64 || image.Inode == 0 || image.Inode > math.MaxInt64 || seen[key] {
			return ErrPlan
		}
		previous = image.SHA256
		seen[key] = true
	}
	data, err := json.Marshal(guestStorageImagesIntent{Version: 1, Plan: qualified, Images: images})
	if err != nil {
		return err
	}
	return e.commitImmutableGuestIntent(ctx, "guest-storage-images-intent.json", "guest-storage-images-intent", data)
}
