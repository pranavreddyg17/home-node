package install

import (
	"strings"
	"unicode/utf8"
)

// A proposal is not authority to replace host configuration. Its eventual
// publisher must retain the original file, journal both hashes and exclude
// concurrent account allocation before applying the exact reviewed bytes.
type guestIdentityNameServiceProposal struct {
	OriginalSHA256 string `json:"originalSha256"`
	DesiredSHA256  string `json:"desiredSha256"`
	Contents       string `json:"contents"`
}

// Dedicated-host preparation accepts only the currently supported local
// resolver configuration. Remote providers and ambiguous subordinate resolvers
// require review; they must never be silently removed by installation.
func planGuestIdentityNameServices(data []byte) (guestIdentityNameServiceProposal, error) {
	empty := guestIdentityNameServiceProposal{}
	if ValidateNameServices(data) != nil || !utf8.Valid(data) || strings.ContainsAny(string(data), "\r\x00") {
		return empty, ErrAccounts
	}
	lines := strings.Split(string(data), "\n")
	seenSubID := false
	for index, line := range lines {
		body, _, _ := strings.Cut(line, "#")
		name, sources, ok := strings.Cut(body, ":")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		switch name {
		case "passwd", "group", "shadow":
			lines[index] = name + ": files"
		case "subid":
			fields := strings.Fields(sources)
			if seenSubID || len(fields) != 1 || fields[0] != "files" {
				return empty, ErrAccounts
			}
			seenSubID = true
		}
	}
	desired := strings.Join(lines, "\n")
	if !strings.HasSuffix(desired, "\n") {
		desired += "\n"
	}
	if !seenSubID {
		desired += "subid: files\n"
	}
	if len(desired) > maxAccountFileBytes {
		return empty, ErrAccounts
	}
	return guestIdentityNameServiceProposal{OriginalSHA256: digest(data), DesiredSHA256: digest([]byte(desired)), Contents: desired}, nil
}
