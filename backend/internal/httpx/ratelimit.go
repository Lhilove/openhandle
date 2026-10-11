package httpx

import (
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// Limiter is a small in-memory rate limiter keyed by an arbitrary string
// (typically limiter-name + client IP). State is per-process; for multi-instance
// deployments this should be backed by a shared store (documented in README).
type Limiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	limit    rate.Limit
	burst    int
}

type visitor struct {
	lim      *rate.Limiter
	lastSeen time.Time
}

// NewLimiter builds a limiter allowing `perMinute` requests per key per minute
// with a burst equal to `burst`.
func NewLimiter(perMinute int, burst int) *Limiter {
	l := &Limiter{
		visitors: make(map[string]*visitor),
		limit:    rate.Limit(float64(perMinute) / 60.0),
		burst:    burst,
	}
	go l.cleanup()
	return l
}

func (l *Limiter) get(key string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()
	v, ok := l.visitors[key]
	if !ok {
		v = &visitor{lim: rate.NewLimiter(l.limit, l.burst)}
		l.visitors[key] = v
	}
	v.lastSeen = time.Now()
	return v.lim
}

func (l *Limiter) cleanup() {
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for range t.C {
		l.mu.Lock()
		for k, v := range l.visitors {
			if time.Since(v.lastSeen) > 10*time.Minute {
				delete(l.visitors, k)
			}
		}
		l.mu.Unlock()
	}
}

// Middleware returns a Gin middleware enforcing the limit per client IP.
func (l *Limiter) Middleware(name string) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := name + ":" + c.ClientIP()
		if !l.get(key).Allow() {
			Fail(c, 429, CodeRateLimited, "Too many requests. Please try again shortly.")
			return
		}
		c.Next()
	}
}
