package api

import (
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	maxJSONBodyBytes    = 64 * 1024 // 64 KB
	loginWindow         = 5 * time.Minute
	loginMaxFailures    = 10
	rateLimiterMaxKeys  = 4096
	cspHeaderValue      = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src 'self' data: https://fonts.gstatic.com; img-src 'self' data: blob:; media-src 'self' blob:; connect-src 'self' blob:; worker-src 'self' blob:; frame-ancestors 'self'; base-uri 'self'; form-action 'self'"
)

func withSecurityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "SAMEORIGIN")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		h.Set("Content-Security-Policy", cspHeaderValue)

		if isMutatingMethod(r.Method) && !isSameOriginRequest(r) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "跨站請求已遭拒絕"})
			return
		}

		next.ServeHTTP(w, r)
	})
}

func isMutatingMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch:
		return true
	default:
		return false
	}
}

func isSameOriginRequest(r *http.Request) bool {
	if sfs := strings.TrimSpace(r.Header.Get("Sec-Fetch-Site")); sfs != "" {
		if strings.EqualFold(sfs, "same-origin") || strings.EqualFold(sfs, "none") {
			return true
		}
		return false
	}

	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}

	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" {
		return false
	}
	if strings.EqualFold(parsed.Host, r.Host) {
		return true
	}
	if xfh := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); xfh != "" {
		firstHost := strings.TrimSpace(strings.Split(xfh, ",")[0])
		if firstHost != "" && strings.EqualFold(parsed.Host, firstHost) {
			return true
		}
	}
	return false
}

func isHTTPSRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if xfp := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); xfp != "" {
		firstProto := strings.TrimSpace(strings.Split(xfp, ",")[0])
		if strings.EqualFold(firstProto, "https") {
			return true
		}
	}
	return false
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	return json.NewDecoder(r.Body).Decode(dst)
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func isSafeStaticPath(baseDir, targetPath string) bool {
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return false
	}
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absBase, absTarget)
	if err != nil {
		return false
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return false
	}
	return true
}

type loginRateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	window   time.Duration
	maxFails int
}

func newLoginRateLimiter(window time.Duration, maxFails int) *loginRateLimiter {
	return &loginRateLimiter{
		attempts: make(map[string][]time.Time),
		window:   window,
		maxFails: maxFails,
	}
}

func (l *loginRateLimiter) pruneLocked(key string, now time.Time) []time.Time {
	cutoff := now.Add(-l.window)
	list := l.attempts[key]
	kept := list[:0]
	for _, ts := range list {
		if ts.After(cutoff) {
			kept = append(kept, ts)
		}
	}
	if len(kept) == 0 {
		delete(l.attempts, key)
		return nil
	}
	l.attempts[key] = kept
	return kept
}

func (l *loginRateLimiter) Allow(ip, username string) (bool, time.Duration) {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	keys := []string{"ip:" + ip}
	if u := strings.ToLower(strings.TrimSpace(username)); u != "" {
		keys = append(keys, "usr:"+u)
	}
	for _, k := range keys {
		list := l.pruneLocked(k, now)
		if len(list) >= l.maxFails {
			retryAfter := l.window - now.Sub(list[0])
			if retryAfter < time.Second {
				retryAfter = time.Second
			}
			return false, retryAfter
		}
	}
	return true, 0
}

func (l *loginRateLimiter) RecordFailure(ip, username string) {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.attempts) > rateLimiterMaxKeys {
		for k := range l.attempts {
			l.pruneLocked(k, now)
		}
		if len(l.attempts) > rateLimiterMaxKeys {
			l.attempts = make(map[string][]time.Time)
		}
	}

	ipKey := "ip:" + ip
	l.attempts[ipKey] = append(l.pruneLocked(ipKey, now), now)
	if u := strings.ToLower(strings.TrimSpace(username)); u != "" {
		uKey := "usr:" + u
		l.attempts[uKey] = append(l.pruneLocked(uKey, now), now)
	}
}

func (l *loginRateLimiter) RecordSuccess(ip, username string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if u := strings.ToLower(strings.TrimSpace(username)); u != "" {
		delete(l.attempts, "usr:"+u)
	}
	delete(l.attempts, "ip:"+ip)
}
