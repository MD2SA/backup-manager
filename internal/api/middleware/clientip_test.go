package middleware

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestClientIP_DirectConnection(t *testing.T) {
	var got string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = GetClientIP(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	middleware := ClientIP(nil)(handler)

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.100:5678"
	w := httptest.NewRecorder()

	middleware.ServeHTTP(w, req)

	if got != "192.168.1.100" {
		t.Errorf("expected 192.168.1.100, got %q", got)
	}
}

func TestClientIP_WithTrustedProxy(t *testing.T) {
	var got string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = GetClientIP(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	prefixes := []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
	}
	middleware := ClientIP(prefixes)(handler)

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:5678"
	req.Header.Set("X-Forwarded-For", "203.0.113.50, 10.0.0.1")
	w := httptest.NewRecorder()

	middleware.ServeHTTP(w, req)

	if got != "203.0.113.50" {
		t.Errorf("expected client IP 203.0.113.50, got %q", got)
	}
}

func TestClientIP_TrustedProxyButNoXFF(t *testing.T) {
	var got string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = GetClientIP(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	prefixes := []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
	}
	middleware := ClientIP(prefixes)(handler)

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:5678"
	w := httptest.NewRecorder()

	middleware.ServeHTTP(w, req)

	if got != "10.0.0.1" {
		t.Errorf("expected proxy IP 10.0.0.1 when no XFF, got %q", got)
	}
}

func TestClientIP_UntrustedSourceIgnoredXFF(t *testing.T) {
	var got string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = GetClientIP(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	prefixes := []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
	}
	middleware := ClientIP(prefixes)(handler)

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:5678"
	req.Header.Set("X-Forwarded-For", "203.0.113.50")
	w := httptest.NewRecorder()

	middleware.ServeHTTP(w, req)

	if got != "192.168.1.1" {
		t.Errorf("expected remote addr 192.168.1.1 for untrusted source, got %q", got)
	}
}

func TestWalkXFF_AllTrusted(t *testing.T) {
	prefixes := []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
	}
	headers := []string{"10.0.0.2, 10.0.0.1"}
	ip := walkXFF(headers, prefixes)
	if !ip.IsValid() {
		t.Fatal("expected a valid IP even when all entries are trusted")
	}
	if ip.String() != "10.0.0.1" {
		t.Errorf("expected closest (rightmost) trusted proxy 10.0.0.1, got %s", ip)
	}
}

func TestWalkXFF_EmptyHeaders(t *testing.T) {
	ip := walkXFF(nil, nil)
	if ip.IsValid() {
		t.Errorf("expected invalid IP for empty headers, got %s", ip)
	}
}
