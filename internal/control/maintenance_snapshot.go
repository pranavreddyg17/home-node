package control

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/pranavreddyg17/home-node/internal/state"
)

// writeMaintenanceSnapshot builds a sanitized SQLite backup in controller-owned
// temporary storage. No peer-selected path or live database is exposed.
func (s *Server) writeMaintenanceSnapshot(ctx context.Context, token string, w http.ResponseWriter) (resultErr error) {
	var database string
	if err := s.Store.DB.QueryRowContext(ctx, "SELECT file FROM pragma_database_list WHERE name='main'").Scan(&database); err != nil {
		return err
	}
	if !filepath.IsAbs(database) || filepath.Clean(database) != database {
		return state.ErrRecovery
	}
	directory, err := os.MkdirTemp(filepath.Dir(database), "backup-snapshot-")
	if err != nil {
		return err
	}
	// The controller exclusively owns this private directory for the call.
	defer func() { resultErr = errors.Join(resultErr, os.Remove(directory)) }()
	path, err := s.Store.MaintenanceRecoverySnapshot(ctx, token, directory)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, os.Remove(path)) }()
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() <= 0 || info.Size() > 256<<20 {
		return state.ErrRecovery
	}
	if _, err = state.ValidateRecoverySnapshot(ctx, file); err != nil {
		return err
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/vnd.homenode.recovery-snapshot")
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	buffer := make([]byte, 64<<10)
	reader := io.NewSectionReader(file, 0, info.Size())
	remaining := info.Size()
	for remaining > 0 {
		if err = ctx.Err(); err != nil {
			return err
		}
		n, err := io.ReadFull(reader, buffer[:min(remaining, int64(len(buffer)))])
		if err != nil {
			return err
		}
		written, err := w.Write(buffer[:n])
		if err != nil {
			return err
		}
		if written != n {
			return io.ErrShortWrite
		}
		remaining -= int64(n)
	}
	return ctx.Err()
}

type snapshotResponse struct {
	http.ResponseWriter
	started bool
}

func (w *snapshotResponse) Write(data []byte) (int, error) {
	w.started = true
	return w.ResponseWriter.Write(data)
}
