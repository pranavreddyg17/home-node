//go:build linux

package main

import (
	"encoding/json"
	"errors"
	"os"
	"strconv"

	"golang.org/x/sys/unix"
)

// Hand an expiring invitation to the disposable fixture's inherited write pipe.
// Never export a reusable session, credential private key or recovery code.
func writeNativeBrowserInvitation(descriptor, invitation string) error {
	fd, err := strconv.Atoi(descriptor)
	if err != nil || fd < 3 || fd > 4096 || strconv.Itoa(fd) != descriptor || len(invitation) < 20 || len(invitation) > 256 {
		return errors.New("invalid native browser invitation handoff")
	}
	var metadata unix.Stat_t
	if err := unix.Fstat(fd, &metadata); err != nil || metadata.Mode&unix.S_IFMT != unix.S_IFIFO || int(metadata.Uid) != os.Geteuid() {
		return errors.New("browser handoff requires an owned pipe")
	}
	flags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
	if err != nil || flags&unix.O_ACCMODE != unix.O_WRONLY {
		return errors.New("browser handoff requires a write pipe")
	}
	output := os.NewFile(uintptr(fd), "native-browser-invitation")
	if output == nil {
		return errors.New("native browser pipe unavailable")
	}
	defer output.Close()
	return json.NewEncoder(output).Encode(struct {
		Invitation string `json:"invitation"`
	}{invitation})
}
