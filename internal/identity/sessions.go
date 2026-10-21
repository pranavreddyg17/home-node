package identity

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// SessionSummary deliberately has no credential, token hash or reusable handle.
// Device revocation remains the approved way to terminate that device's access.
type SessionSummary struct {
	DeviceID   string `json:"deviceId"`
	CreatedAt  int64  `json:"createdAt"`
	VerifiedAt int64  `json:"verifiedAt"`
	LastSeen   int64  `json:"lastSeen"`
	ExpiresAt  int64  `json:"expiresAt"`
	Current    bool   `json:"current"`
}

func (s *Service) Sessions(ctx context.Context, actor Session) ([]SessionSummary, error) {
	if !actor.Allows("admin") {
		return nil, ErrDenied
	}
	summaries := []SessionSummary{}
	err := s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		if err := checkActor(tx, actor); err != nil {
			return err
		}
		now := time.Now().Unix()
		rows, err := tx.Query(`SELECT s.device_id,s.created_at,s.verified_at,s.last_seen,s.expires_at,s.token_hash=?
FROM sessions s JOIN devices d ON d.id=s.device_id JOIN identity i ON i.epoch=s.epoch
WHERE d.revoked_at IS NULL AND s.expires_at>? AND s.last_seen>?
ORDER BY s.created_at,s.device_id,s.token_hash LIMIT 1281`, actor.TokenHash, now, now-1800)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var summary SessionSummary
			if err := rows.Scan(&summary.DeviceID, &summary.CreatedAt, &summary.VerifiedAt, &summary.LastSeen, &summary.ExpiresAt, &summary.Current); err != nil {
				return err
			}
			summaries = append(summaries, summary)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		// Enrollment permits 64 active devices and login permits 20 sessions each.
		// Refuse corrupt excess state rather than silently hiding active sessions.
		if len(summaries) > 1280 {
			return errors.New("active session inventory exceeds enrollment bounds")
		}
		return nil
	})
	return summaries, err
}
