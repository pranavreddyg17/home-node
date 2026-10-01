package state

import (
	"context"
	"errors"
)

// ObserveSchemaVersion reads the live database version without running schema
// migrations. Update compatibility must use this observation rather than a
// browser value or an installation-time constant. Admission must remain closed
// across the eventual migration to keep this observation stable.
func (s *Store) ObserveSchemaVersion(ctx context.Context) (int, error) {
	if s == nil || s.DB == nil {
		return 0, errors.New("state database unavailable")
	}
	var version int
	if err := s.DB.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return 0, err
	}
	if version < 1 || version > 1024 {
		return 0, errors.New("state schema is outside update compatibility bounds")
	}
	return version, nil
}
