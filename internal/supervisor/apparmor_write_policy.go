package supervisor

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
)

func (d Domain) AppArmorName() string { return "libvirt-homenode-" + d.ID }

// AppArmorWritePolicy adds deny rules for the complement of a finite write
// allowlist. Denies override writable paths inherited from distro abstractions.
// It renders policy only; loading, ownership, launch and lifecycle are separate.
func (d Domain) AppArmorWritePolicy() (string, error) {
	if !guestproto.ValidID(d.ID) || d.GuestUID < 65536 || d.GuestGID == 0 {
		return "", ErrPolicy
	}
	for _, path := range []string{d.SystemPath, d.DataPath, d.ChannelPath} {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path || path == "/" || len(path) > 256 || strings.Trim(path, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789/._-") != "" {
			return "", ErrPolicy
		}
	}
	writable := []string{d.DataPath, d.ChannelPath, "/dev/kvm", "/dev/null", "/dev/zero"}
	allowed := make(map[string]bool)
	for _, path := range writable {
		if path == d.SystemPath || allowed[path] {
			return "", ErrPolicy
		}
		allowed[path] = true
	}
	type node struct {
		terminal bool
		children map[byte]*node
	}
	root := &node{children: map[byte]*node{}}
	for _, path := range writable {
		current := root
		for i := 1; i < len(path); i++ {
			ch := path[i]
			if current.children[ch] == nil {
				current.children[ch] = &node{children: map[byte]*node{}}
			}
			current = current.children[ch]
		}
		current.terminal = true
	}
	var out strings.Builder
	fmt.Fprintf(&out, "#include <tunables/global>\nprofile %s flags=(attach_disconnected) {\n #include <abstractions/libvirt-qemu>\n", d.AppArmorName())
	fmt.Fprintf(&out, " \"%s\" rk,\n \"%s\" rwk,\n \"%s\" rw,\n", d.SystemPath, d.DataPath, d.ChannelPath)
	var emit func(string, *node)
	emit = func(prefix string, current *node) {
		if !current.terminal {
			fmt.Fprintf(&out, " deny \"%s\" wkl,\n", prefix)
		}
		if current.children['/'] == nil && !strings.HasSuffix(prefix, "/") {
			fmt.Fprintf(&out, " deny \"%s/**\" wkl,\n", prefix)
		}
		keys := make([]int, 0, len(current.children))
		var excluded strings.Builder
		excluded.WriteByte('/')
		for ch := range current.children {
			keys = append(keys, int(ch))
		}
		sort.Ints(keys)
		for _, k := range keys {
			if byte(k) != '/' && byte(k) != '-' {
				excluded.WriteByte(byte(k))
			}
		}
		if current.children['-'] != nil {
			excluded.WriteString(`\-`)
		}
		fmt.Fprintf(&out, " deny \"%s[^%s]**\" wkl,\n", prefix, excluded.String())
		for _, k := range keys {
			emit(prefix+string(byte(k)), current.children[byte(k)])
		}
	}
	emit("/", root)
	out.WriteString("}\n")
	if out.Len() > 256<<10 {
		return "", ErrPolicy
	}
	return out.String(), nil
}
