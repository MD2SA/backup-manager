package middleware

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

type clientIPContextKey struct{}

// clientIPKey is the context key storing the resolved client IP.
var clientIPKey = &clientIPContextKey{}

// ClientIP stores the real client IP, honouring X-Forwarded-For when the
// connection comes from a trusted proxy. Falls back to the TCP RemoteAddr when
// no trusted proxy is in play, so direct deployments keep working.
func ClientIP(trustedPrefixes []netip.Prefix) func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			remote, ok := remoteIP(r.RemoteAddr)
			if ok && ipInPrefixes(remote, trustedPrefixes) {
				// Request arrived via a trusted proxy: walk XFF right-to-left,
				// skipping trusted hops, to recover the true client IP.
				if client := walkXFF(r.Header.Values("X-Forwarded-For"), trustedPrefixes); client.IsValid() {
					setClientIP(r, client)
					h.ServeHTTP(w, r)
					return
				}
			}

			setClientIP(r, remote)
			h.ServeHTTP(w, r)
		})
	}
}

// GetClientIP reads the client IP previously stored by the ClientIP middleware.
func GetClientIP(ctx context.Context) string {
	ip, _ := ctx.Value(clientIPKey).(netip.Addr)
	if !ip.IsValid() {
		return ""
	}
	return ip.String()
}

// setClientIP writes the resolved client IP into a package-local context key.
func setClientIP(r *http.Request, ip netip.Addr) {
	if !ip.IsValid() {
		return
	}
	*r = *r.WithContext(context.WithValue(r.Context(), clientIPKey, ip.Unmap()))
}

// remoteIP parses the host portion of a TCP RemoteAddr (e.g. "1.2.3.4:5678").
func remoteIP(remoteAddr string) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	if host == "" {
		return netip.Addr{}, false
	}
	ip, err := netip.ParseAddr(strings.TrimSpace(host))
	if err != nil {
		return netip.Addr{}, false
	}
	return ip.Unmap(), true
}

// walkXFF walks a merged XFF chain right-to-left, returning the first IP not
// falling within a trusted prefix.
func walkXFF(headers []string, trusted []netip.Prefix) netip.Addr {
	var entries []string
	for i := len(headers) - 1; i >= 0; i-- {
		parts := strings.Split(headers[i], ",")
		for j := len(parts) - 1; j >= 0; j-- {
			if e := strings.TrimSpace(parts[j]); e != "" {
				entries = append(entries, e)
			}
		}
	}

	for _, e := range entries {
		ip, ok := remoteIP(e)
		if !ok {
			return netip.Addr{} // fail-closed on garbage
		}
		if ipInPrefixes(ip, trusted) {
			continue
		}
		return ip
	}

	if len(entries) > 0 {
		if ip, ok := remoteIP(entries[0]); ok {
			return ip
		}
	}
	return netip.Addr{}
}

// ipInPrefixes reports whether the address falls within any of the prefixes.
func ipInPrefixes(ip netip.Addr, prefixes []netip.Prefix) bool {
	for _, p := range prefixes {
		if p.Contains(ip.Unmap()) {
			return true
		}
	}
	return false
}
