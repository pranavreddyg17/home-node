//go:build !linux

package runtimeclient

import "github.com/pranavreddyg17/home-node/internal/backup"

func validateBackupCredentialSocket(string, uint32) error { return backup.ErrManifest }
