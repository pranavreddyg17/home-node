package supervisor

import (
	"errors"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestDomainAppArmorIdentity(t *testing.T) {
	d := Domain{ID: state.Random()}
	uuid := "12345678-abcd-4123-8123-123456789abc"
	if err := validateDomainAppArmorIdentity(d, uuid, "libvirt-"+uuid+" (enforce)\n"); err != nil {
		t.Fatal(err)
	}
	for _, change := range []string{"foreign-label", "suffix", "complain", "empty", "invalid-uuid", "uppercase", "short", "separator", "invalid-id", "static-uuid"} {
		t.Run(change, func(t *testing.T) {
			domain, id, label := d, uuid, "libvirt-"+uuid+" (enforce)"
			switch change {
			case "foreign-label":
				label = "libvirt-aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa (enforce)"
			case "suffix":
				label += " extra"
			case "complain":
				label = strings.Replace(label, "enforce", "complain", 1)
			case "empty":
				label = ""
			case "invalid-uuid":
				id = strings.Replace(id, "1", "g", 1)
				label = "libvirt-" + id + " (enforce)"
			case "uppercase":
				id = strings.ToUpper(id)
				label = "libvirt-" + id + " (enforce)"
			case "short":
				id = id[:35]
				label = "libvirt-" + id + " (enforce)"
			case "separator":
				id = strings.Replace(id, "-", "a", 1)
				label = "libvirt-" + id + " (enforce)"
			case "invalid-id":
				domain.ID = "../foreign"
			case "static-uuid":
				domain.ConfinedWrites = true
			}
			if err := validateDomainAppArmorIdentity(domain, id, label); !errors.Is(err, ErrPolicy) {
				t.Fatal("foreign profile identity admitted", err)
			}
		})
	}
	d.ConfinedWrites = true
	if err := validateDomainAppArmorIdentity(d, d.RuntimeUUID(), d.AppArmorName()+" (enforce)"); err != nil {
		t.Fatal(err)
	}
}
