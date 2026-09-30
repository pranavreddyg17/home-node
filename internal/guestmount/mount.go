// Package guestmount admits the guest data mount before application startup.
package guestmount

import (
	"errors"
	"strings"
)

var ErrMount = errors.New("guest data mount admission failed")

func admit(data string, device string) error {
	if len(data) > 1<<20 {
		return ErrMount
	}
	found := false
	for _, line := range strings.Split(data, "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 10 {
			return ErrMount
		}
		if fields[4] != "/data" {
			continue
		}
		if found {
			return ErrMount
		}
		found = true
		separator := -1
		for i := 6; i < len(fields); i++ {
			if fields[i] == "-" {
				separator = i
				break
			}
		}
		if separator < 6 || len(fields) != separator+4 || fields[2] != device || fields[3] != "/" || fields[separator+1] != "ext4" {
			return ErrMount
		}
		options := map[string]bool{}
		for _, option := range strings.Split(fields[5], ",") {
			options[option] = true
		}
		for _, required := range []string{"rw", "nodev", "nosuid", "noexec"} {
			if !options[required] {
				return ErrMount
			}
		}
		if options["ro"] {
			return ErrMount
		}
		writable := false
		for _, option := range strings.Split(fields[separator+3], ",") {
			if option == "ro" {
				return ErrMount
			}
			if option == "rw" {
				writable = true
			}
		}
		if !writable {
			return ErrMount
		}
	}
	if !found {
		return ErrMount
	}
	return nil
}
