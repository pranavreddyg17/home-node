package supervisor

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"
)

// validateGuestUIDNameServices qualifies explicitly local-only identity and
// subordinate-ID resolution. Dynamic systemd/NSS identities cannot establish
// an exclusively provisioned numeric pool. Other databases are immaterial.
func validateGuestUIDNameServices(ctx context.Context, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(data) == 0 || len(data) > 1<<20 || !utf8.Valid(data) || strings.ContainsAny(string(data), "\r\x00") {
		return ErrPolicy
	}
	required := map[string]bool{"passwd": false, "group": false, "shadow": false, "subid": false}
	for _, line := range strings.Split(string(data), "\n") {
		if err := ctx.Err(); err != nil {
			return err
		}
		line, _, _ = strings.Cut(line, "#")
		if strings.TrimSpace(line) == "" {
			continue
		}
		name, body, ok := strings.Cut(line, ":")
		if !ok {
			return ErrPolicy
		}
		name = strings.TrimSpace(name)
		seen, relevant := required[name]
		if !relevant {
			continue
		}
		fields := strings.Fields(body)
		if seen {
			return fmt.Errorf("duplicate %s identity resolution rule: %w", name, ErrPolicy)
		}
		if len(fields) != 1 || fields[0] != "files" {
			return fmt.Errorf("%s identity resolution requires an explicit files-only rule: %w", name, ErrPolicy)
		}
		required[name] = true
	}
	for _, name := range []string{"passwd", "group", "shadow", "subid"} {
		if !required[name] {
			return fmt.Errorf("missing explicit %s identity resolution rule: %w", name, ErrPolicy)
		}
	}
	return ctx.Err()
}
