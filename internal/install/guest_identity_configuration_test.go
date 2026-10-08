package install

import (
	"errors"
	"strings"
	"testing"
)

func TestGuestIdentityNameServiceProposalPreservesUnrelatedResolution(t *testing.T) {
	input := []byte("# dedicated host\npasswd: files systemd\ngroup: files systemd\nshadow: files\nhosts: files dns\nservices: db files\n")
	before := string(input)
	proposal, err := planGuestIdentityNameServices(input)
	if err != nil || proposal.OriginalSHA256 != digest(input) || proposal.DesiredSHA256 != digest([]byte(proposal.Contents)) || string(input) != before {
		t.Fatal("proposal lost source binding", err)
	}
	if proposal.Contents != "# dedicated host\npasswd: files\ngroup: files\nshadow: files\nhosts: files dns\nservices: db files\nsubid: files\n" {
		t.Fatal("unrelated resolution changed", proposal.Contents)
	}
	retry, err := planGuestIdentityNameServices([]byte(proposal.Contents))
	if err != nil || retry.Contents != proposal.Contents || retry.OriginalSHA256 != proposal.DesiredSHA256 {
		t.Fatal("prepared bytes are not stable", err)
	}
	for _, bad := range []string{
		strings.Replace(before, "passwd: files systemd", "passwd: files sss", 1),
		before + "subid: sss\n", before + "subid: files\nsubid: files\n",
		before + "\r", before + "\x00",
	} {
		result, err := planGuestIdentityNameServices([]byte(bad))
		if !errors.Is(err, ErrAccounts) || result != (guestIdentityNameServiceProposal{}) {
			t.Fatal("ambiguous host resolver produced replacement bytes", err)
		}
	}
}
