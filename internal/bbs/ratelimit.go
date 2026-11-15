package bbs

import (
	"sync"
	"time"
)

type rateBucket struct {
	count   int
	resetAt time.Time
}

type rateLimiter struct {
	mu        sync.Mutex
	buckets   map[string]*rateBucket
	max       int
	window    time.Duration
	lastSweep time.Time
}

func newRateLimiter(max int, window time.Duration) *rateLimiter {
	if max <= 0 {
		max = 5
	}
	if window <= 0 {
		window = time.Minute
	}
	return &rateLimiter{
		buckets:   make(map[string]*rateBucket),
		max:       max,
		window:    window,
		lastSweep: time.Now(),
	}
}

// Allow 返回是否允许这次尝试。超过上限返回 false。
func (r *rateLimiter) Allow(key string) bool {
	if key == "" {
		return true
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()

	// 每分钟清理一次过期 bucket
	if now.Sub(r.lastSweep) > time.Minute {
		for k, b := range r.buckets {
			if now.After(b.resetAt) {
				delete(r.buckets, k)
			}
		}
		r.lastSweep = now
	}

	b, ok := r.buckets[key]
	if !ok || now.After(b.resetAt) {
		r.buckets[key] = &rateBucket{count: 1, resetAt: now.Add(r.window)}
		return true
	}
	if b.count >= r.max {
		return false
	}
	b.count++
	return true
}

// Reset 清除某个 key 的计数（登录成功后调用）。
func (r *rateLimiter) Reset(key string) {
	if key == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.buckets, key)
}
