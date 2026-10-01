package backup

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/pranavreddyg17/home-node/internal/state"
)

var ErrMaintenanceRunner = errors.New("maintenance runner is already active or unavailable")

func claimMaintenanceRunner(ctx context.Context, store *state.Store) (*os.File, error) {
	var path string
	if err := store.DB.QueryRowContext(ctx, "SELECT file FROM pragma_database_list WHERE name='main'").Scan(&path); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, ErrMaintenanceRunner
	}
	return lockMaintenanceRunner(ctx, filepath.Dir(path))
}
