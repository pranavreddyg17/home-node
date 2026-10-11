package networkcheck

import (
	"crypto/tls"
	"sync"
	"time"
)

// CertificateSource observes protected certificate replacement at most once a
// minute. A bad replacement fails new handshakes rather than silently retaining
// an old identity. It does not obtain certificates or keep tailnet-admin tokens.
type CertificateSource struct {
	mu          sync.Mutex
	load        func() (tls.Certificate, error)
	now         func() time.Time
	checked     time.Time
	certificate tls.Certificate
	err         error
}

func NewCertificateSource(c Config, certPath, keyPath string) (*CertificateSource, error) {
	return newCertificateSource(c, certPath, keyPath, nil)
}

// NewGatewayCertificateSource also pins key access to the gateway's private
// group. A bridge-group key could otherwise expose it to the controller.
func NewGatewayCertificateSource(c Config, certPath, keyPath string, gatewayGID uint32) (*CertificateSource, error) {
	if gatewayGID == 0 {
		return nil, ErrIdentity
	}
	return newCertificateSource(c, certPath, keyPath, &gatewayGID)
}

func newCertificateSource(c Config, certPath, keyPath string, group *uint32) (*CertificateSource, error) {
	s := &CertificateSource{load: func() (tls.Certificate, error) { return inspectWithKeyGroup(c, certPath, keyPath, group) }, now: time.Now}
	if _, err := s.GetCertificate(nil); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *CertificateSource) GetCertificate(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if s.checked.IsZero() || now.Sub(s.checked) >= time.Minute || now.Before(s.checked) {
		s.certificate, s.err = s.load()
		s.checked = now
	}
	if s.err != nil {
		return nil, s.err
	}
	if s.certificate.Leaf == nil || !now.Before(s.certificate.Leaf.NotAfter) || now.Before(s.certificate.Leaf.NotBefore) {
		return nil, ErrIdentity
	}
	// Return a copy so future replacement cannot modify an in-flight handshake.
	certificate := s.certificate
	return &certificate, nil
}
