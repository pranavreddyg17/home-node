//go:build !linux

package backup

import (
	"context"
	"errors"
	"os"
)

var ErrRepository = errors.New("encrypted repository could not be authenticated")

func VerifyRepository(context.Context, *os.File, Target, []byte) error { return ErrRepository }
