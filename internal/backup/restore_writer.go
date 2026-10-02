package backup

import (
	"context"
	"io"
	"os"
)

type restoreWriter struct {
	ctx         context.Context
	stage       *os.File
	destination io.Writer
	remaining   int64
}

func (w *restoreWriter) Write(data []byte) (int, error) {
	if w.ctx == nil {
		return 0, ErrManifest
	}
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	if int64(len(data)) > w.remaining {
		return 0, ErrManifest
	}
	if err := requireStagingSpace(w.stage, w.remaining); err != nil {
		return 0, err
	}
	n, err := w.destination.Write(data)
	w.remaining -= int64(n)
	return n, err
}
