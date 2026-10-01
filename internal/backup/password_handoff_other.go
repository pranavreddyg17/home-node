//go:build !linux

package backup

import (
	"context"
	"os"
)

func CreateRepositoryPassword([]byte) (*os.File, error)                { return nil, ErrRepository }
func ReadRepositoryPassword(context.Context, *os.File) ([]byte, error) { return nil, ErrRepository }
