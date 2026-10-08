package install

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

type guestUIDAllocatorRanges struct {
	NormalFirst, NormalLast           uint32
	SystemFirst, SystemLast           uint32
	SubordinateFirst, SubordinateLast uint32
}

type guestUIDAllocationProposal struct {
	OriginalSHA256 string `json:"originalSha256"`
	DesiredSHA256  string `json:"desiredSha256"`
	Contents       string `json:"contents"`
}

// Selection is explicit policy, never inferred from commented or absent distro
// defaults. This prepares bytes only; it neither excludes writers nor reserves
// identities. A publisher must retain host, account and runtime authority.
func planGuestUIDAllocatorConfiguration(ctx context.Context, original []byte, pool supervisor.GuestUIDPool, selection guestUIDAllocatorRanges) (guestUIDAllocationProposal, error) {
	empty := guestUIDAllocationProposal{}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if len(original) == 0 || len(original) > 8192 || !utf8.Valid(original) || strings.ContainsAny(string(original), "\r\x00") {
		return empty, ErrPlan
	}
	names := []string{"UID_MIN", "UID_MAX", "SYS_UID_MIN", "SYS_UID_MAX", "SUB_UID_MIN", "SUB_UID_MAX"}
	values := []uint32{selection.NormalFirst, selection.NormalLast, selection.SystemFirst, selection.SystemLast, selection.SubordinateFirst, selection.SubordinateLast}
	replacements := map[string]string{}
	var explicit strings.Builder
	for i, name := range names {
		replacements[name] = fmt.Sprintf("%s %d", name, values[i])
		explicit.WriteString(replacements[name] + "\n")
	}
	if err := supervisor.ValidateGuestUIDAutomaticAllocationConfiguration(ctx, pool, []byte(explicit.String())); err != nil {
		return empty, err
	}
	seen := map[string]bool{}
	lines := strings.SplitAfter(string(original), "\n")
	var desired strings.Builder
	for _, line := range lines {
		if err := ctx.Err(); err != nil {
			return empty, err
		}
		declaration, _, _ := strings.Cut(line, "#")
		fields := strings.Fields(declaration)
		if len(fields) == 0 {
			desired.WriteString(line)
			continue
		}
		replacement, relevant := replacements[fields[0]]
		if !relevant {
			desired.WriteString(line)
			continue
		}
		if len(fields) != 2 || seen[fields[0]] {
			return empty, ErrConflict
		}
		// Refuse malformed active declarations even when a replacement was chosen.
		for _, digit := range fields[1] {
			if digit < '0' || digit > '9' {
				return empty, ErrConflict
			}
		}
		if _, err := strconv.ParseUint(fields[1], 10, 32); err != nil {
			return empty, ErrConflict
		}
		seen[fields[0]] = true
		desired.WriteString(replacement + "\n")
	}
	if desired.Len() > 0 && !strings.HasSuffix(desired.String(), "\n") {
		desired.WriteByte('\n')
	}
	for _, name := range names {
		if !seen[name] {
			desired.WriteString(replacements[name] + "\n")
		}
	}
	contents := desired.String()
	if len(contents) > 8192 {
		return empty, ErrPlan
	}
	if err := supervisor.ValidateGuestUIDAutomaticAllocationConfiguration(ctx, pool, []byte(contents)); err != nil {
		return empty, err
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	return guestUIDAllocationProposal{OriginalSHA256: digest(original), DesiredSHA256: digest([]byte(contents)), Contents: contents}, nil
}
