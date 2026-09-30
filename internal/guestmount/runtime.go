package guestmount

import "strings"

// admitRuntimeMount binds admission to the mount opened by the adapter rather
// than an unrelated hidden mount at the same path.
func admitRuntimeMount(data, path, device, mountID string) error {
	if len(data) > 1<<20 || (path != "/tmp" && path != "/var") {
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
		if fields[0] != mountID {
			continue
		}
		if found || fields[2] != device || (fields[3] != "/" && path != "/tmp") || fields[4] != path {
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
		if separator < 6 || len(fields) != separator+4 || fields[separator+1] != "tmpfs" {
			return ErrMount
		}
	}
	if !found {
		return ErrMount
	}
	return nil
}
