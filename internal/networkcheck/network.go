// Package networkcheck validates the private HTTPS identity before a controller
// listener is opened. It does not certify tailnet access policy or reachability
// from another device; onboarding must verify those separately.
package networkcheck

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"time"
)

var ErrIdentity = errors.New("private HTTPS identity is invalid")
var tailnet = netip.MustParsePrefix("100.64.0.0/10")

type Config struct {
	Bind   string
	Port   int
	Origin string
}

// Validate checks configuration, local interface ownership and the complete
// server chain without sending the private key or making network requests.
func Validate(c Config, local []netip.Addr, certPEM, keyPEM []byte, roots *x509.CertPool, now time.Time) (tls.Certificate, error) {
	var empty tls.Certificate
	ip, err := netip.ParseAddr(c.Bind)
	if err != nil || !tailnet.Contains(ip) || c.Port < 1024 || c.Port > 65535 {
		return empty, fmt.Errorf("bind must be a local Tailscale IPv4 address with an unprivileged port: %w", ErrIdentity)
	}
	found := false
	for _, addr := range local {
		if addr.Unmap() == ip {
			found = true
		}
	}
	if !found {
		return empty, fmt.Errorf("configured Tailscale address is not assigned to this host: %w", ErrIdentity)
	}
	origin, err := url.Parse(c.Origin)
	if err != nil || origin.Scheme != "https" || origin.Host == "" || origin.User != nil || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" || origin.ForceQuery || origin.Opaque != "" || origin.String() != c.Origin {
		return empty, fmt.Errorf("origin must be an exact HTTPS origin: %w", ErrIdentity)
	}
	host := origin.Hostname()
	if host == "" || net.ParseIP(host) != nil {
		return empty, fmt.Errorf("origin needs a stable DNS name: %w", ErrIdentity)
	}
	port := 443
	if origin.Port() != "" {
		port, err = strconv.Atoi(origin.Port())
		if err != nil {
			return empty, ErrIdentity
		}
	}
	if port != c.Port {
		return empty, fmt.Errorf("origin port differs from listener port: %w", ErrIdentity)
	}
	certificate, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil || len(certificate.Certificate) == 0 {
		return empty, fmt.Errorf("certificate and key do not form a valid pair: %w", ErrIdentity)
	}
	leaf, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil {
		return empty, ErrIdentity
	}
	intermediates := x509.NewCertPool()
	for _, der := range certificate.Certificate[1:] {
		intermediate, err := x509.ParseCertificate(der)
		if err != nil {
			return empty, ErrIdentity
		}
		intermediates.AddCert(intermediate)
	}
	if _, err = leaf.Verify(x509.VerifyOptions{DNSName: host, Roots: roots, Intermediates: intermediates, CurrentTime: now, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}); err != nil {
		return empty, fmt.Errorf("certificate must be trusted, current and valid for the configured origin: %w", ErrIdentity)
	}
	certificate.Leaf = leaf
	return certificate, nil
}

func LocalAddresses() ([]netip.Addr, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var addresses []netip.Addr
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		values, err := iface.Addrs()
		if err != nil {
			return nil, err
		}
		for _, value := range values {
			prefix, err := netip.ParsePrefix(value.String())
			if err != nil {
				continue
			}
			addresses = append(addresses, prefix.Addr())
		}
	}
	return addresses, nil
}
