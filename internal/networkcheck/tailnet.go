package networkcheck

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/netip"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

const maxStatusBytes = 1 << 20

type statusOutput struct{ bytes.Buffer }

func (b *statusOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > maxStatusBytes {
		return 0, ErrIdentity
	}
	return b.Buffer.Write(p)
}

// Decode only the current host identity. Never retain or return peer/user data.
// Tailscale documents status JSON as subject to change: missing identity fields
// must therefore fail closed, rather than guessing a different local interface.
func validateTailnet(c Config, data []byte) error {
	if len(data) > maxStatusBytes {
		return ErrIdentity
	}
	var status struct {
		BackendState string
		TailscaleIPs []netip.Addr
		Self         *struct{ DNSName string }
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&status); err != nil {
		return ErrIdentity
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return ErrIdentity
	}
	origin, err := url.Parse(c.Origin)
	if err != nil || status.BackendState != "Running" || status.Self == nil || status.Self.DNSName == "" || !strings.EqualFold(strings.TrimSuffix(status.Self.DNSName, "."), origin.Hostname()) {
		return fmt.Errorf("running Tailscale host DNS identity differs from the origin: %w", ErrIdentity)
	}
	bind, err := netip.ParseAddr(c.Bind)
	if err != nil {
		return ErrIdentity
	}
	for _, addr := range status.TailscaleIPs {
		if addr == bind {
			return nil
		}
	}
	return fmt.Errorf("bind address is not this node's Tailscale address: %w", ErrIdentity)
}

func inspectTailnet(c Config) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/tailscale", "status", "--json", "--peers=false")
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "LC_ALL=C"}
	cmd.WaitDelay = time.Second
	var output statusOutput
	cmd.Stdout = &output
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("cannot read running Tailscale identity; check the local client/daemon: %w", ErrIdentity)
	}
	return validateTailnet(c, output.Bytes())
}
