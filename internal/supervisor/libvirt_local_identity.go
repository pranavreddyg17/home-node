package supervisor

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// Caller authenticates local passwd bytes independently; no NSS/network lookup.
// A package-owned runtime directory is still only the location of a PID hint.
func localLibvirtQEMUIdentity(passwd []byte) (uint32, uint32, error) {
	if len(passwd) == 0 || len(passwd) > 1<<20 || !utf8.Valid(passwd) || strings.ContainsAny(string(passwd), "\x00\r") {
		return 0, 0, ErrPolicy
	}
	counts := map[uint32]int{}
	var uid, gid uint32
	found := false
	for _, line := range strings.Split(strings.TrimSuffix(string(passwd), "\n"), "\n") {
		fields := strings.Split(line, ":")
		if len(fields) != 7 || fields[0] == "" {
			return 0, 0, ErrPolicy
		}
		u, err := decimalUID(fields[2])
		if err != nil {
			return 0, 0, err
		}
		g, err := decimalUID(fields[3])
		if err != nil {
			return 0, 0, err
		}
		if strconv.FormatUint(uint64(u), 10) != fields[2] || strconv.FormatUint(uint64(g), 10) != fields[3] {
			return 0, 0, ErrPolicy
		}
		counts[u]++
		if fields[0] == "libvirt-qemu" {
			if found || u == 0 || u >= 65536 || g == 0 || g > 1<<31-1 {
				return 0, 0, ErrPolicy
			}
			found = true
			uid, gid = u, g
		}
	}
	if !found || counts[uid] != 1 {
		return 0, 0, ErrPolicy
	}
	return uid, gid, nil
}

func qualifyDomainPIDDirectory(uid, gid, mode, guestUID, serviceUID, serviceGID uint32) error {
	if mode&0170000 != 0040000 || mode&0022 != 0 {
		return ErrPolicy
	}
	if uid == 0 {
		return nil
	}
	if serviceUID == 0 || serviceUID >= 65536 || serviceGID == 0 || serviceGID > 1<<31-1 || uid != serviceUID || gid != serviceGID || uid == guestUID {
		return ErrPolicy
	}
	return nil
}
