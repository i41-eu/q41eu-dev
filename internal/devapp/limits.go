package devapp

// Limits for a server on the internet: actions per key (or paired browser), wrong pairing codes
// and page streams per address. Over a limit the answer is 429 with what the limit is.

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	actionsPerSecond = 10 // per key or browser: show, say, leds, … (a card a second is plenty)
	actionsBurst     = 30
	pairFailsPer10m  = 20 // wrong pairing codes per address in 10 minutes
	streamsPerAddr   = 20 // open page event streams per address
)

// buckets is a token bucket per key: rate tokens a second up to burst.
type buckets struct {
	mu    sync.Mutex
	rate  float64
	burst float64
	m     map[string]*bucket
}

type bucket struct {
	tokens float64
	last   time.Time
}

func newBuckets(rate, burst float64) *buckets {
	return &buckets{rate: rate, burst: burst, m: map[string]*bucket{}}
}

// take spends one token for key; false when there is none.
func (b *buckets) take(key string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	if len(b.m) > 10000 { // forget idle ones (they are full again anyway)
		for k, v := range b.m {
			if now.Sub(v.last).Seconds()*b.rate > b.burst {
				delete(b.m, k)
			}
		}
	}
	x := b.m[key]
	if x == nil {
		x = &bucket{tokens: b.burst, last: now}
		b.m[key] = x
	}
	x.tokens = min(b.burst, x.tokens+now.Sub(x.last).Seconds()*b.rate)
	x.last = now
	if x.tokens < 1 {
		return false
	}
	x.tokens--
	return true
}

// has says whether key has a token left, without spending one.
func (b *buckets) has(key string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	x := b.m[key]
	if x == nil {
		return true
	}
	return min(b.burst, x.tokens+time.Since(x.last).Seconds()*b.rate) >= 1
}

// clientIP is the browser's address: behind TrustedProxies reverse proxies, the one they saw
// (X-Forwarded-For, from the right); else the connection's.
func (a *App) clientIP(r *http.Request) string {
	if n := a.cfg.TrustedProxies; n > 0 {
		var hops []string
		for _, v := range r.Header.Values("X-Forwarded-For") {
			for _, p := range strings.Split(v, ",") {
				if p = strings.TrimSpace(p); p != "" {
					hops = append(hops, p)
				}
			}
		}
		if len(hops) >= n {
			return hops[len(hops)-n]
		}
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
