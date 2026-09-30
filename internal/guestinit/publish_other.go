//go:build !linux

package guestinit

import "os"

func publishIntent(*os.Root, string) error { return ErrDirectory }
