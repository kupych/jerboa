package server

import (
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// SecurityHeadersMiddleware sets a conservative set of security response headers.
// CSP is intentionally permissive for inline styles/scripts because the SPA bundles
// them via Vite; tighten only once the build emits hashes/nonces.
func SecurityHeadersMiddleware(secure bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			h.Set("Permissions-Policy", "camera=(), geolocation=(), microphone=(self), payment=(), usb=()")
			if secure {
				h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}

// rateLimiter is a very small fixed-window limiter keyed by IP address.
// Used for public, unauthenticated endpoints (magic link send, login redirect)
// to stop trivial abuse. Not a replacement for a real WAF.
type rateLimiter struct {
	mu       sync.Mutex
	hits     map[string]*bucket
	limit    int
	window   time.Duration
}

type bucket struct {
	count int
	reset time.Time
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{
		hits:   make(map[string]*bucket),
		limit:  limit,
		window: window,
	}
}

func (rl *rateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	b, ok := rl.hits[key]
	if !ok || now.After(b.reset) {
		rl.hits[key] = &bucket{count: 1, reset: now.Add(rl.window)}
		// Opportunistic GC: drop up to 32 stale buckets on write.
		if len(rl.hits) > 256 {
			i := 0
			for k, v := range rl.hits {
				if now.After(v.reset) {
					delete(rl.hits, k)
				}
				i++
				if i > 32 {
					break
				}
			}
		}
		return true
	}
	if b.count >= rl.limit {
		return false
	}
	b.count++
	return true
}

// RateLimitMiddleware limits requests per IP to `limit` per `window`.
// Relies on middleware.RealIP having run earlier so r.RemoteAddr is trustworthy.
func RateLimitMiddleware(limit int, window time.Duration) func(http.Handler) http.Handler {
	rl := newRateLimiter(limit, window)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := r.RemoteAddr
			if colon := strings.LastIndex(ip, ":"); colon > 0 {
				ip = ip[:colon]
			}
			if !rl.allow(ip) {
				w.Header().Set("Retry-After", "60")
				http.Error(w, `{"error":"rate limited"}`, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// isPrivateBaseURL reports whether baseURL points at a private / local network.
// Used to gate the dev-auth bypass: flipping JERBOA_DEV_AUTH=true on a
// production .env shouldn't do anything unless BaseURL is also pointed at
// a non-public address — and changing BaseURL in prod would break OIDC
// redirects and emails, so the attack is self-defeating.
//
// Accepted: localhost, loopback IPs, RFC1918 private ranges, and *.local
// (mDNS) — which covers typical LAN testing from phones/tablets.
func isPrivateBaseURL(baseURL string) bool {
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme != "http" {
		return false // https:// is treated as public
	}
	host := u.Hostname()
	if host == "" {
		return false
	}
	if host == "localhost" || strings.HasSuffix(host, ".local") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false // unresolved hostname — refuse
	}
	return ip.IsLoopback() || ip.IsPrivate()
}

// sanitizeHeaderValue removes characters that could enable header injection
// (CR, LF) and trims surrounding whitespace. Also strips quotes/backslashes
// so the value is safe inside a `filename="..."` parameter.
func sanitizeHeaderValue(v string) string {
	v = strings.TrimSpace(v)
	replacer := strings.NewReplacer(
		"\r", "",
		"\n", "",
		"\x00", "",
		"\"", "",
		"\\", "",
	)
	v = replacer.Replace(v)
	if len(v) > 200 {
		v = v[:200]
	}
	return v
}
