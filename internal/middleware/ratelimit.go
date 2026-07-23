package middleware

// RateLimit is an in-memory, per-client-IP token-bucket limiter exposed as a
// gorilla/mux MiddlewareFunc so it can be mounted on any subrouter.
//
//	RateLimit(requestsPerMinute int, burst int) mux.MiddlewareFunc
//
// The same constructor drives both a strict auth limiter and a looser checkout
// limiter - the caller picks the numbers. Suggested wiring (done by the
// integrator in cmd/main.go, NOT here):
//
//	auth subrouter (login/register): RateLimit(10, 10)   // ~10 req/min, brute-force brake
//	orders subrouter (checkout/pay): RateLimit(30, 30)   // ~30 req/min, burst-friendly
//
// Client IP: derived from r.RemoteAddr only. X-Forwarded-For is deliberately
// NOT trusted by default - honoring it without a vetted trusted-proxy hop lets a
// client forge the key and evade the limit. Behind a trusted L7 proxy, unwrap
// XFF's first hop at the edge (or extend this file with an explicit trusted-proxy
// allowlist) before relying on it.
//
// Memory is bounded by a lazy sweep: idle buckets are evicted on a periodic pass
// guarded by the same mutex, so a burst of unique IPs cannot grow the map without
// bound once those IPs go quiet.

import (
	"encoding/json"
	"math"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/mux"
)

// bucketTTL is how long an idle bucket is kept before the sweep evicts it. Any
// value comfortably above one refill window works; the bucket is fully refilled
// long before then, so eviction never wrongly penalizes a returning client.
const bucketTTL = 10 * time.Minute

// sweepInterval bounds how often the lazy cleanup scans the map. The scan runs at
// most once per interval, piggybacked on an incoming request (no background goroutine).
const sweepInterval = time.Minute

type tokenBucket struct {
	tokens   float64
	lastSeen time.Time
}

type rateLimiter struct {
	mu         sync.Mutex
	buckets    map[string]*tokenBucket
	ratePerSec float64 // tokens refilled per second
	burst      float64 // bucket capacity
	lastSweep  time.Time
	nowFn      func() time.Time // injectable for tests
}

// RateLimit returns a mux.MiddlewareFunc that admits at most requestsPerMinute
// sustained requests per client IP, allowing short spikes up to burst. On refusal
// it answers 429 with a JSON body and a Retry-After header (whole seconds).
func RateLimit(requestsPerMinute int, burst int) mux.MiddlewareFunc {
	if requestsPerMinute < 1 {
		requestsPerMinute = 1
	}
	if burst < 1 {
		burst = 1
	}
	rl := &rateLimiter{
		buckets:    make(map[string]*tokenBucket),
		ratePerSec: float64(requestsPerMinute) / 60.0,
		burst:      float64(burst),
		nowFn:      time.Now,
	}
	rl.lastSweep = rl.nowFn()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)
			ok, retryAfter := rl.allow(ip)
			if !ok {
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "rate limit exceeded"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// allow consumes one token for ip. It reports whether the request is admitted and,
// when refused, how many whole seconds until a token frees up (Retry-After).
func (rl *rateLimiter) allow(ip string) (bool, int) {
	now := rl.nowFn()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.sweepLocked(now)

	b, ok := rl.buckets[ip]
	if !ok {
		// A fresh client starts full, then spends one token for this request.
		rl.buckets[ip] = &tokenBucket{tokens: rl.burst - 1, lastSeen: now}
		return true, 0
	}

	// Refill by elapsed time, capped at burst, then try to spend one token.
	elapsed := now.Sub(b.lastSeen).Seconds()
	b.tokens = math.Min(rl.burst, b.tokens+elapsed*rl.ratePerSec)
	b.lastSeen = now

	if b.tokens >= 1 {
		b.tokens -= 1
		return true, 0
	}

	// Seconds until the bucket regains a full token.
	retry := int(math.Ceil((1 - b.tokens) / rl.ratePerSec))
	if retry < 1 {
		retry = 1
	}
	return false, retry
}

// sweepLocked evicts idle buckets. Caller must hold rl.mu. It runs at most once per
// sweepInterval so the scan cost is amortized across many requests.
func (rl *rateLimiter) sweepLocked(now time.Time) {
	if now.Sub(rl.lastSweep) < sweepInterval {
		return
	}
	rl.lastSweep = now
	for ip, b := range rl.buckets {
		if now.Sub(b.lastSeen) > bucketTTL {
			delete(rl.buckets, ip)
		}
	}
}

// clientIP extracts the host portion of RemoteAddr. See the package-level note on
// why X-Forwarded-For is not trusted here.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// RemoteAddr had no port (or was already a bare host) - use it verbatim.
		return r.RemoteAddr
	}
	return host
}
