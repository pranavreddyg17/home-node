package supervisor

import (
	"errors"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestDomainAppArmorWritePolicyRejectsInjectionAndAuthorityAliases(t *testing.T) {
	for _, kind := range []string{"wildcard", "quote", "newline", "relative", "traversal", "readonly-alias", "channel-alias", "device-alias", "missing-uid", "oversized"} {
		t.Run(kind, func(t *testing.T) {
			d := Domain{ID: state.Random(), GuestUID: 200000, GuestGID: 108, SystemPath: "/var/lib/homenode/images/system.raw", DataPath: "/var/lib/homenode/volumes/video.raw", ChannelPath: "/run/homenode/video/adapter.sock"}
			switch kind {
			case "wildcard":
				d.DataPath = "/var/lib/homenode/*"
			case "quote":
				d.DataPath = "/var/lib/homenode/\"injection"
			case "newline":
				d.DataPath = "/var/lib/homenode/\ninjection"
			case "relative":
				d.DataPath = "relative.raw"
			case "traversal":
				d.DataPath = "/var/lib/homenode/../foreign"
			case "readonly-alias":
				d.SystemPath = d.DataPath
			case "channel-alias":
				d.ChannelPath = d.DataPath
			case "device-alias":
				d.DataPath = "/dev/kvm"
			case "missing-uid":
				d.GuestUID = 0
			case "oversized":
				d.DataPath = "/" + strings.Repeat("a", 256)
			}
			if policy, err := d.AppArmorWritePolicy(); !errors.Is(err, ErrPolicy) || policy != "" {
				t.Fatal("untrusted policy admitted", kind, err)
			}
		})
	}
}

func TestDomainAppArmorStaticSelectorBindsProtectedInstanceName(t *testing.T) {
	d := Domain{ID: state.Random(), GuestUID: 200000, GuestGID: 108, SystemPath: "/var/lib/homenode/images/system.raw", DataPath: "/var/lib/homenode/volumes/video.raw", ChannelPath: "/run/homenode/video/adapter.sock", ConfinedWrites: true}
	d.Image.MemoryMiB = 512
	d.Image.VCPUs = 1
	xml, err := d.XML()
	if err != nil {
		t.Fatal(err)
	}
	if !guestproto.ValidID(d.ID) || !strings.Contains(xml, "<label>"+d.AppArmorName()+"</label>") || strings.Contains(xml, "type='dynamic' model='apparmor'") {
		t.Fatal("static label not bound")
	}
	policy, err := d.AppArmorWritePolicy()
	if err != nil || !strings.Contains(policy, "profile "+d.AppArmorName()+" flags=(attach_disconnected)") {
		t.Fatal("profile not bound", err)
	}
}
