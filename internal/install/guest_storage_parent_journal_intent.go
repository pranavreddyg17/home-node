package install

import (
	"context"
	"encoding/json"
	"strings"
)

type guestStorageParentJournalIntent struct {
	Version            int     `json:"version"`
	ParentIntentSHA256 string  `json:"parentIntentSha256"`
	Original           journal `json:"original"`
	Desired            journal `json:"desired"`
}

// Caller retains authenticated parent provenance and installed journal under
// migration exclusion. Record both states durably before any parent mutation.
func (e *Engine) commitGuestStorageParentJournalIntent(ctx context.Context, installed journal, sourceGID, guestGID uint32, parentSHA string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(parentSHA) != 64 || strings.Trim(parentSHA, "0123456789abcdef") != "" || len(installed.ID) != 32 || strings.Trim(installed.ID, "0123456789abcdef") != "" || len(installed.Items) == 0 || len(installed.Items) > maxInstallationItems {
		return ErrPlan
	}
	desired, err := planGuestStorageImageParentJournal(ctx, installed, sourceGID, guestGID)
	if err != nil {
		return err
	}
	data, err := json.Marshal(guestStorageParentJournalIntent{Version: 1, ParentIntentSHA256: parentSHA, Original: installed, Desired: desired})
	if err != nil {
		return err
	}
	return e.commitImmutableGuestIntent(ctx, "guest-storage-image-parent-journal.json", "guest-storage-image-parent-journal", data)
}
