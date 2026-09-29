package supervisor

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/catalog"
	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestDomainAgainstInstalledLibvirtSchema(t *testing.T) {
	if os.Getenv("HOMENODE_DOMAIN_SCHEMA_TEST") != "1" {
		t.Skip("requires the supported distribution's libvirt XML schema")
	}
	tool, err := exec.LookPath("virt-xml-validate")
	if err != nil {
		t.Fatal(err)
	}
	domain := Domain{ID: state.Random(), Image: catalog.Image{MemoryMiB: 512, VCPUs: 1}, SystemPath: "/var/lib/homenode/images/image.raw", DataPath: "/var/lib/homenode/volumes/data.raw", ChannelPath: "/run/homenode/guests/instance/adapter.sock"}
	xml, err := domain.XML()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "domain.xml")
	if err = os.WriteFile(path, []byte(xml), 0600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(tool, path, "domain").CombinedOutput(); err != nil {
		t.Fatalf("invalid domain XML: %v\n%s", err, output)
	}
}
