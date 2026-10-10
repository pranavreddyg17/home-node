package gatewaytransport

import (
	"context"
	"net"
	"net/http"
	"net/netip"

	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

type httpsKey struct{}

// TrustedHTTPS is true only after the private socket's kernel peer identity and
// the gateway's bounded transport assertions have been verified. Public
// forwarding headers can never create this context value.
func TrustedHTTPS(r *http.Request) bool {
	trusted, _ := r.Context().Value(httpsKey{}).(bool)
	return trusted
}

func ControllerHandler(gatewayUID uint32, next http.Handler) (http.Handler, error) {
	if gatewayUID == 0 || next == nil {
		return nil, ErrTransport
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w = &deadlineWriter{ResponseWriter: w}
		uid, ok := supervisor.RequestPeerUID(r)
		addresses, versions := r.Header.Values(clientAddressHeader), r.Header.Values(clientTLSHeader)
		if !ok || uid != gatewayUID || len(addresses) != 1 || len(versions) != 1 || versions[0] != "1.3" {
			http.Error(w, "gateway peer denied", http.StatusForbidden)
			return
		}
		address, err := netip.ParseAddr(addresses[0])
		if err != nil || !tailnet.Contains(address) || address.String() != addresses[0] {
			http.Error(w, "gateway assertion denied", http.StatusForbidden)
			return
		}
		r = r.Clone(context.WithValue(r.Context(), httpsKey{}, true))
		r.RemoteAddr = net.JoinHostPort(address.String(), "0")
		r.Header.Del(clientAddressHeader)
		r.Header.Del(clientTLSHeader)
		next.ServeHTTP(w, r)
	}), nil
}
