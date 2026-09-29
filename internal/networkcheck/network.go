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
	"regexp"
	"strconv"
	"strings"
	"time"
)

var ErrIdentity = errors.New("private HTTPS identity is invalid")
var tailnet = netip.MustParsePrefix("100.64.0.0/10")
var dnsLabel = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

type Config struct {
	Bind   string
	Port   int
	Origin string
}

// Validate checks configuration, local interface ownership and the complete
// server chain without sending the private key or making network requests.
func Validate(c Config, local []netip.Addr, certPEM, keyPEM []byte, roots *x509.CertPool, now time.Time) (tls.Certificate, error) {

	var empty tls.Certificate
	host, err := ValidateConfiguration(c)
	if err != nil {
		return empty, err
	}
	ip, _ := netip.ParseAddr(c.Bind)
	found := false
	for _, addr := range local {
		if addr.Unmap() == ip {
			found = true
		}
	}
	if !found {
		return empty, fmt.Errorf("configured Tailscale address is not assigned to this host: %w", ErrIdentity)
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

// ValidateConfiguration checks deterministic values suitable for a plan or
// systemd environment file. It does not observe actual network/TLS readiness.
func ValidateConfiguration(c Config) (string, error) {
	ip, err := netip.ParseAddr(c.Bind)
	if err != nil || !tailnet.Contains(ip) || c.Port < 1024 || c.Port > 65535 {
		return "", fmt.Errorf("bind needs a Tailscale IPv4 address and unprivileged port: %w", ErrIdentity)
	}
	origin, err := url.Parse(c.Origin)
	if err != nil || origin.Scheme != "https" || origin.Host == "" || origin.User != nil || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" || origin.ForceQuery || origin.Opaque != "" {
		return "", ErrIdentity
	}
	host := origin.Hostname()
	if len(host) > 253 || !strings.Contains(host, ".") || net.ParseIP(host) != nil {
		return "", ErrIdentity
	}
	for _, part := range strings.Split(host, ".") {
		if !dnsLabel.MatchString(part) {
			return "", ErrIdentity
		}
	}
	if c.Origin != "https://"+net.JoinHostPort(host, strconv.Itoa(c.Port)) {
		return "", fmt.Errorf("origin must use its canonical DNS name and exact listener port: %w", ErrIdentity)
	}
	return host, nil
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
