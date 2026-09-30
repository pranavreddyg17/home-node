//go:build !linux

package supervisor

import "context"

func prepareDataVolume(context.Context, string, int64, int64) error { return ErrPolicy }

func purgeVolumePreparation(context.Context, string, string, int64) error { return ErrPolicy }
