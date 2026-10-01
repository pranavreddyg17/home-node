// Package disktransport transfers one read-only regular-file descriptor with a
// bounded opaque metadata packet. Callers must authenticate kernel peers and
// validate metadata, ownership, and maintenance authority separately.
package disktransport

import "errors"

const MaxPacket = 4096

var ErrPacket = errors.New("maintenance descriptor packet rejected")
