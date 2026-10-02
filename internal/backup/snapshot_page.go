package backup

import "context"

const snapshotPageSize = 25

// SnapshotPage contains metadata candidates, never a recovery authorization.
// Next is a full snapshot ID; a stale cursor requires refreshing the inventory.
type SnapshotPage struct {
	Snapshots []SnapshotReference `json:"snapshots"`
	Next      string              `json:"next"`
}

func (r *Repository) SnapshotPage(ctx context.Context, cursor string) (SnapshotPage, error) {
	if cursor != "" && !repositoryPattern.MatchString(cursor) {
		return SnapshotPage{}, ErrRepository
	}
	candidates, err := r.Snapshots(ctx)
	if err != nil {
		return SnapshotPage{}, err
	}
	page, err := pageSnapshotCandidates(candidates, cursor)
	if err != nil {
		return SnapshotPage{}, err
	}
	if err := ctx.Err(); err != nil {
		return SnapshotPage{}, err
	}
	return page, nil
}

func pageSnapshotCandidates(candidates []SnapshotReference, cursor string) (SnapshotPage, error) {
	if len(candidates) > maxSnapshotInventoryEntries || (cursor != "" && !repositoryPattern.MatchString(cursor)) {
		return SnapshotPage{}, ErrRepository
	}
	start := 0
	if cursor != "" {
		found := false
		for index, candidate := range candidates {
			if candidate.ID == cursor {
				start = index + 1
				found = true
				break
			}
		}
		if !found {
			return SnapshotPage{}, ErrRepository
		}
	}
	end := start + snapshotPageSize
	if end > len(candidates) {
		end = len(candidates)
	}
	page := SnapshotPage{Snapshots: append([]SnapshotReference{}, candidates[start:end]...)}
	if end < len(candidates) {
		page.Next = candidates[end-1].ID
	}
	return page, nil
}
