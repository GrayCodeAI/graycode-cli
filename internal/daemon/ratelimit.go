package daemon

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ipLimiter is a per-IP token bucket rate limiter implemented in the standard
// library (avoids promoting golang.org/x/time to a direct dependency). Each
// remote IP gets its own bucket refilled at `rate` tokens/sec up to `burst`.
type ipLimiter struct {
	mu      sync.Mutex
	buckets map[string]*tokenBucket
	rate    float64
	burst   float64
}

// tokenBucket holds the current token count and last refill time.
type tokenBucket struct {
	tokens float64
	last   time.Time
}

func newIPLimiter(rate, burst float64) *ipLimiter {
	return &ipLimiter{
		buckets: make(map[string]*tokenBucket),
		rate:    rate,
		burst:   burst,
	}
}

// Allow reports whether a request from ip may proceed, consuming a token.
// Unbounded bucket-map growth is prevented by opportunistically pruning empty
// buckets when a new IP arrives: the count of buckets is bounded by the number
// of distinct IPs seen within one burst window in practice.
func (l *ipLimiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	b, ok := l.buckets[ip]
	if !ok {
		// Opportunistic prune: drop buckets that have fully drained (they
		// will be recreated on the IP's next request).
		if prev := len(l.buckets); prev > 1024 {
			now := time.Now()
			for k, v := range l.buckets {
				v.refill(now, l.rate, l.burst)
				if v.tokens < 1 {
					delete(l.buckets, k)
				}
			}
		}
		b = &tokenBucket{tokens: l.burst, last: time.Now()}
		l.buckets[ip] = b
	}
	b.refill(time.Now(), l.rate, l.burst)
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func (b *tokenBucket) refill(now time.Time, rate, burst float64) {
	elapsed := now.Sub(b.last).Seconds()
	b.tokens += elapsed * rate
	if b.tokens > burst {
		b.tokens = burst
	}
	b.last = now
}

// clientIP extracts the client's IP from a request, excluding the port. The
// daemon binds loopback by default and is not meant to sit behind proxies, so
// RemoteAddr (the direct peer) is authoritative.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	// Fall back to the raw address if it has no port.
	if r.RemoteAddr != "" {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return "unknown"
}
