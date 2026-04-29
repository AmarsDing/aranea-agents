package middleware

import (
	"net/http"
	"sync"
	"time"
)

type visitor struct {
	lastSeen time.Time
	tokens   int
}

func RateLimit(maxPerMinute int) func(http.Handler) http.Handler {
	var mu sync.Mutex
	visitors := map[string]*visitor{}

	go func() {
		ticker := time.NewTicker(2 * time.Minute)
		for range ticker.C {
			mu.Lock()
			for ip, v := range visitors {
				if time.Since(v.lastSeen) > 5*time.Minute {
					delete(visitors, ip)
				}
			}
			mu.Unlock()
		}
	}()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.RemoteAddr
			now := time.Now()

			mu.Lock()
			v, ok := visitors[key]
			if !ok {
				v = &visitor{lastSeen: now, tokens: maxPerMinute}
				visitors[key] = v
			}
			if now.Sub(v.lastSeen) >= time.Minute {
				v.tokens = maxPerMinute
			}
			v.lastSeen = now
			if v.tokens <= 0 {
				mu.Unlock()
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			v.tokens--
			mu.Unlock()

			next.ServeHTTP(w, r)
		})
	}
}
