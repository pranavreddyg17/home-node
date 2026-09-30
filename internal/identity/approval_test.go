package identity

import (
	"strings"
	"testing"
)

func TestApprovalBindingCoversAuthorityAndCopiesResources(t *testing.T) {
	actor := Session{Device: Device{ID: "owner-device"}, Epoch: 1, TokenHash: strings.Repeat("a", 64)}
	resources := []string{"video", "files"}
	b, err := newApprovalBinding(actor, "app.stop", resources, []byte(`{"action":"stop"}`), 1, 1000)
	if err != nil {
		t.Fatal(err)
	}
	resources[0] = "ai"
	if b.Resources[1] != "video" {
		t.Fatal("binding aliased caller resources")
	}
	other, err := newApprovalBinding(actor, "app.stop", []string{"files", "video"}, []byte(`{"action":"stop"}`), 1, 1000)
	if err != nil || other.digest() != b.digest() {
		t.Fatal("resource order changed authority", err)
	}
	changes := []ApprovalBinding{b, b, b, b, b, b, b, b}
	changes[0].Action = "app.start"
	changes[1].Resources = []string{"ai"}
	changes[2].BodySHA256 = strings.Repeat("b", 64)
	changes[3].PolicyGeneration++
	changes[4].Epoch++
	changes[5].ExpiresAt--
	changes[6].DeviceID = "another-device"
	changes[7].SessionHash = strings.Repeat("c", 64)
	for i, c := range changes {
		if c.digest() == b.digest() {
			t.Fatal("authority change not bound", i)
		}
	}
}

func TestApprovalBindingRejectsInvalidOrExpiredAuthority(t *testing.T) {
	actor := Session{Device: Device{ID: "owner-device"}, Epoch: 1, TokenHash: strings.Repeat("a", 64)}
	b, err := newApprovalBinding(actor, "device.revoke", []string{"target"}, nil, 1, 1000)
	if err != nil {
		t.Fatal(err)
	}
	cases := []ApprovalBinding{b, b, b, b, b, b, b, b, b, b}
	cases[0].Action = "shell.execute"
	cases[1].Resources = []string{"target", "target"}
	cases[2].Resources = []string{"../target"}
	cases[3].PolicyGeneration = 0
	cases[4].Epoch = 0
	cases[5].ExpiresAt = 1000
	cases[6].ExpiresAt = 1121
	cases[7].BodySHA256 = strings.Repeat("A", 64)
	cases[8].SessionHash = "missing"
	cases[9].DeviceID = ""
	for i, c := range cases {
		if c.valid(1000) {
			t.Fatal("invalid binding accepted", i)
		}
	}
	if b.valid(1120) || b.valid(999) {
		t.Fatal("expired binding or rollback accepted")
	}
}
