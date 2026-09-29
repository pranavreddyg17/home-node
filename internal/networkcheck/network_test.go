package networkcheck

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/netip"
	"testing"
	"time"
)

func testIdentity(t *testing.T, now time.Time) ([]byte, []byte, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "fixture root"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(48 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	rootDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	root, err := x509.ParseCertificate(rootDER)
	if err != nil {
		t.Fatal(err)
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), DNSNames: []string{"home.example.ts.net"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	leafDER, err := x509.CreateCertificate(rand.Reader, leaf, root, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	private, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(root)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: private}), roots
}

func TestPrivateHTTPSIdentity(t *testing.T) {
	now := time.Now()
	cert, key, roots := testIdentity(t, now)
	config := Config{Bind: "100.100.1.2", Port: 8787, Origin: "https://home.example.ts.net:8787"}
	local := []netip.Addr{netip.MustParseAddr(config.Bind)}
	result, err := Validate(config, local, cert, key, roots, now)
	if err != nil || result.Leaf == nil {
		t.Fatal(err)
	}
	cases := []struct {
		name      string
		config    Config
		local     []netip.Addr
		cert, key []byte
		roots     *x509.CertPool
		now       time.Time
	}{
		{"nonlocal address", config, nil, cert, key, roots, now},
		{"expired", config, local, cert, key, roots, now.Add(2 * time.Hour)},
		{"not yet valid", config, local, cert, key, roots, now.Add(-2 * time.Hour)},
		{"untrusted", config, local, cert, key, x509.NewCertPool(), now},
		{"wrong key", config, local, cert, []byte("invalid"), roots, now},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Validate(tc.config, tc.local, tc.cert, tc.key, tc.roots, tc.now); err == nil {
				t.Fatal("unsafe identity accepted")
			}
		})
	}
	for _, origin := range []string{"http://home.example.ts.net:8787", "https://home.example.ts.net", "https://other.example.ts.net:8787", "https://home.example.ts.net:8787/", "https://home.example.ts.net:8787?", "https://home.example.ts.net:8787#", "https://user@home.example.ts.net:8787", "https://100.100.1.2:8787"} {
		c := config
		c.Origin = origin
		if _, err := Validate(c, local, cert, key, roots, now); err == nil {
			t.Fatal("invalid origin accepted", origin)
		}
	}
	for _, bind := range []string{"0.0.0.0", "127.0.0.1", "192.168.1.2", "::1"} {
		c := config
		c.Bind = bind
		if _, err := Validate(c, []netip.Addr{netip.MustParseAddr(bind)}, cert, key, roots, now); err == nil {
			t.Fatal("public/LAN bind accepted", bind)
		}
	}
}
