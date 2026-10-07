package supervisor

import (
	"path"
	"strings"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
)

// guestCgroupScope identifies the specific systemd scope containing this domain.
// The workload slice itself is never a substitute for a per-domain memory bound.
func guestCgroupScope(relative, id string) (string, error) {
	if !guestproto.ValidID(id) || path.Clean(relative) != relative {
		return "", ErrPolicy
	}
	parts := strings.Split(relative, "/")
	if len(parts) < 3 || len(parts) > 19 || parts[0] != "" || parts[1] != "homenode.slice" {
		return "", ErrPolicy
	}
	scope := parts[2]
	prefix := `machine-qemu\x2d`
	suffix := `\x2dhomenode\x2d` + id + ".scope"
	if !strings.HasPrefix(scope, prefix) || !strings.HasSuffix(scope, suffix) {
		return "", ErrPolicy
	}
	ordinal := strings.TrimSuffix(strings.TrimPrefix(scope, prefix), suffix)
	if ordinal == "" || len(ordinal) > 10 || ordinal[0] == '0' {
		return "", ErrPolicy
	}
	for _, digit := range ordinal {
		if digit < '0' || digit > '9' {
			return "", ErrPolicy
		}
	}
	for _, component := range parts[3:] {
		if component == "" || len(component) > 255 || strings.ContainsAny(component, "\\\x00\r\n") {
			return "", ErrPolicy
		}
	}
	return "/homenode.slice/" + scope, nil
}
