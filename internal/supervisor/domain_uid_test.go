package supervisor

import (
	"github.com/pranavreddyg17/home-node/internal/state"
	"strings"
	"testing"
)

func TestDomainReservedDACIdentityRetainsAppArmor(t *testing.T) {
	m, _ := newManager(t)
	d := Domain{ID: state.Random(), Image: m.Manifest.Images[0], SystemPath: "/images/base.raw", DataPath: "/volumes/data.raw", ChannelPath: "/run/channel.sock", GuestUID: 200000, GuestGID: 64055}
	data, err := d.XML()
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"<label>+200000:+64055</label>", "type='static' model='dac' relabel='no'", "type='dynamic' model='apparmor' relabel='yes'"} {
		if !strings.Contains(data, required) {
			t.Fatal("missing confinement", required)
		}
	}
	for _, identity := range [][2]uint32{{0, 64055}, {65535, 64055}, {200000, 0}, {1 << 31, 64055}, {200000, 1 << 31}} {
		d.GuestUID, d.GuestGID = identity[0], identity[1]
		if _, err = d.XML(); err == nil {
			t.Fatal("invalid DAC identity accepted", identity)
		}
	}
}
