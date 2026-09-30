package supervisor

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"text/template"

	"github.com/pranavreddyg17/home-node/internal/catalog"
	"github.com/pranavreddyg17/home-node/internal/guestproto"
)

type Domain struct {
	ID                                string
	Image                             catalog.Image
	DiskReserveBytes                  int64
	SystemPath, DataPath, ChannelPath string
}

func (d Domain) Name() string { return "homenode-" + d.ID }
func (d Domain) XML() (string, error) {
	if !guestproto.ValidID(d.ID) || d.Image.MemoryMiB < 256 || d.Image.MemoryMiB > 12288 || d.Image.VCPUs < 1 || d.Image.VCPUs > 8 {
		return "", errors.New("invalid domain profile")
	}
	for _, path := range []string{d.SystemPath, d.DataPath, d.ChannelPath} {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return "", errors.New("invalid protected path")
		}
	}
	escape := func(value string) string {
		var buffer bytes.Buffer
		for _, r := range value {
			switch r {
			case '&':
				buffer.WriteString("&amp;")
			case '<':
				buffer.WriteString("&lt;")
			case '>':
				buffer.WriteString("&gt;")
			case '\'':
				buffer.WriteString("&apos;")
			case '"':
				buffer.WriteString("&quot;")
			default:
				buffer.WriteRune(r)
			}
		}
		return buffer.String()
	}
	t, err := template.New("domain").Funcs(template.FuncMap{"xml": escape}).Parse(domainXML)
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	value := struct {
		Domain
		HardMiB, Quota int
	}{d, d.Image.MemoryMiB + 512, d.Image.VCPUs * 100000}
	if err = t.Execute(&out, value); err != nil {
		return "", err
	}
	if out.Len() > 16384 {
		return "", fmt.Errorf("domain XML exceeds limit")
	}
	return out.String(), nil
}

// The template intentionally has no NIC, hostdev, shared filesystem, graphics,
// console, QEMU argument extensions or user-provided XML fragments.
const domainXML = `<domain type='kvm'>
 <name>{{.Name}}</name>
 <memory unit='MiB'>{{.Image.MemoryMiB}}</memory>
 <currentMemory unit='MiB'>{{.Image.MemoryMiB}}</currentMemory>
 <vcpu placement='static'>{{.Image.VCPUs}}</vcpu>
 <resource><partition>/homenode</partition></resource>
 <memtune><hard_limit unit='MiB'>{{.HardMiB}}</hard_limit><swap_hard_limit unit='MiB'>{{.HardMiB}}</swap_hard_limit></memtune>
 <cputune><period>100000</period><quota>{{.Quota}}</quota><emulator_period>100000</emulator_period><emulator_quota>50000</emulator_quota></cputune>
 <memoryBacking><nosharepages/></memoryBacking>
 <os><type arch='x86_64' machine='q35'>hvm</type><boot dev='hd'/></os>
 <features><acpi/><apic/><vmport state='off'/></features>
 <cpu mode='host-model'/>
 <clock offset='utc'/>
 <on_poweroff>destroy</on_poweroff><on_reboot>destroy</on_reboot><on_crash>destroy</on_crash>
 <devices>
  <emulator>/usr/bin/qemu-system-x86_64</emulator>
  <controller type='usb' model='none'/>
  <disk type='file' device='disk'><driver name='qemu' type='raw' cache='none'/><source file='{{xml .SystemPath}}'/><target dev='vda' bus='virtio'/><serial>homenode-system</serial><readonly/><iotune><read_bytes_sec>104857600</read_bytes_sec></iotune></disk>
  <disk type='file' device='disk'><driver name='qemu' type='raw' cache='none'/><source file='{{xml .DataPath}}'/><target dev='vdb' bus='virtio'/><serial>homenode-data</serial><iotune><read_bytes_sec>52428800</read_bytes_sec><write_bytes_sec>52428800</write_bytes_sec></iotune></disk>
  <controller type='virtio-serial' index='0'/>
  <channel type='unix'><source mode='bind' path='{{xml .ChannelPath}}'/><target type='virtio' name='org.homenode.adapter'/></channel>
  <memballoon model='none'/>
  <rng model='virtio'><rate bytes='1024' period='1000'/><backend model='random'>/dev/urandom</backend></rng>
 </devices>
 <seclabel type='dynamic' model='apparmor' relabel='yes'/>
</domain>`
