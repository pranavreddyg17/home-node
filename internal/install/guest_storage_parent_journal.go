package install

import (
	"context"
	"encoding/json"
)

// Input is the authenticated installed journal. This plans exactly one owned
// directory group transition; it neither writes the journal nor mutates storage.
func planGuestStorageImageParentJournal(ctx context.Context, installed journal, sourceGID, guestGID uint32) (journal, error) {
	return planGuestStorageDirectoryGroupJournal(ctx, installed, "var/lib/homenode/images", sourceGID, guestGID)
}

func planGuestStorageVolumeParentJournal(ctx context.Context, installed journal, sourceGID, guestGID uint32) (journal, error) {
	return planGuestStorageDirectoryGroupJournal(ctx, installed, "var/lib/homenode/volumes", sourceGID, guestGID)
}

func planGuestStorageDirectoryGroupJournal(ctx context.Context, installed journal, path string, sourceGID, guestGID uint32) (journal, error) {
	if err := ctx.Err(); err != nil {
		return journal{}, err
	}
	if path != "var/lib/homenode/images" && path != "var/lib/homenode/volumes" {
		return journal{}, ErrPlan
	}
	if installed.Version != 1 || installed.Phase != "installed" || sourceGID == 0 || guestGID == 0 || sourceGID > 1<<31-1 || guestGID > 1<<31-1 {
		return journal{}, ErrPlan
	}
	proposed := installed
	proposed.Items = append([]record(nil), installed.Items...)
	normalized := append([]record(nil), installed.Items...)
	matches := 0
	for i, item := range installed.Items {
		normalized[i].State = "pending"
		if item.Path != path {
			continue
		}
		matches++
		if !item.Directory || item.UID != 0 || item.GID != int(sourceGID) || item.Mode != 0710 || item.SHA256 != "" || (item.State != "created" && item.State != "existing") {
			return journal{}, ErrConflict
		}
		proposed.Items[i].GID = int(guestGID)
	}
	source, err := json.Marshal(normalized)
	if err != nil {
		return journal{}, err
	}
	if matches != 1 || digest(source) != installed.Digest {
		return journal{}, ErrConflict
	}
	for i, item := range proposed.Items {
		normalized[i] = item
		normalized[i].State = "pending"
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return journal{}, err
	}
	proposed.Digest = digest(data)
	return proposed, nil
}
