//go:build !linux

package backup

import (
	"context"
	"errors"
	"os"
)

var ErrRepository = errors.New("encrypted repository could not be authenticated")

func VerifyRepository(context.Context, *os.File, Target, []byte) error { return ErrRepository }

type Repository struct{}

func OpenRepository(context.Context, Target, []byte) (*Repository, error) { return nil, ErrRepository }
func (*Repository) Check(context.Context) error                           { return ErrRepository }
func (*Repository) Close() error                                          { return nil }
