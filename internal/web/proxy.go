package web

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
)

type secureRequestContextKey struct{}

func trustedProxyMiddleware(prefixes []netip.Prefix, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r, forwardedHTTPS := normalizeTrustedProxyRequest(r, prefixes)
		if forwardedHTTPS {
			r = r.WithContext(context.WithValue(r.Context(), secureRequestContextKey{}, true))
		}
		next.ServeHTTP(w, r)
	})
}

func normalizeTrustedProxyRequest(r *http.Request, prefixes []netip.Prefix) (*http.Request, bool) {
	secure := r.TLS != nil
	trusted := remoteAddressTrusted(r.RemoteAddr, prefixes)
	proto := r.Header.Values("X-Forwarded-Proto")
	forwardedHosts := r.Header.Values("X-Forwarded-Host")

	if trusted && len(proto) == 1 && strings.TrimSpace(proto[0]) == "https" {
		host := r.Host
		valid := len(forwardedHosts) == 0 || len(forwardedHosts) == 1 && validForwardedHost(strings.TrimSpace(forwardedHosts[0]))
		if valid && len(forwardedHosts) == 1 {
			host = strings.TrimSpace(forwardedHosts[0])
		}
		if valid && validForwardedHost(host) {
			r = r.Clone(r.Context())
			r.Host = host
			secure = true
		}
	}

	// Header proxy tidak diteruskan agar handler lain tidak menganggapnya tepercaya.
	r = r.Clone(r.Context())
	r.Header = r.Header.Clone()
	r.Header.Del("X-Forwarded-Proto")
	r.Header.Del("X-Forwarded-Host")
	return r, secure
}

func remoteAddressTrusted(remote string, prefixes []netip.Prefix) bool {
	if len(prefixes) == 0 {
		return false
	}
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	address, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	address = address.Unmap()
	for _, prefix := range prefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

func validForwardedHost(host string) bool {
	if host == "" || strings.TrimSpace(host) != host || strings.ContainsAny(host, ",/\\% \t\r\n@") {
		return false
	}
	parsed, err := url.Parse("https://" + host)
	return err == nil && parsed.Host == host && parsed.User == nil && parsed.Path == "" && parsed.RawQuery == "" && parsed.Fragment == "" && parsed.Hostname() != ""
}

func requestIsSecure(r *http.Request, forceSecure bool) bool {
	if forceSecure || r.TLS != nil {
		return true
	}
	secure, _ := r.Context().Value(secureRequestContextKey{}).(bool)
	return secure
}
