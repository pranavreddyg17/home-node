//go:build !linux

package backup

import "os"

func OpenTarget(Target) (*os.File, error) { return nil, ErrTarget }
