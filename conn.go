package main

import "sync"

// connTracker 统计每个用户的活跃已登录连接数。
// 匿名连接（userID == ""）不统计，MVP 不做匿名连接限制。
type connTracker struct {
	mu     sync.Mutex
	counts map[string]int
}

var conns = &connTracker{counts: make(map[string]int)}

// Acquire 尝试为用户占用一个连接槽位。limit <= 0 表示不限制。
func (c *connTracker) Acquire(userID string, limit int) bool {
	if userID == "" || limit <= 0 {
		return true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.counts[userID] >= limit {
		return false
	}
	c.counts[userID]++
	return true
}

func (c *connTracker) Release(userID string) {
	if userID == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.counts[userID] > 1 {
		c.counts[userID]--
	} else {
		delete(c.counts, userID)
	}
}

// Count 仅用于调试/测试
func (c *connTracker) Count(userID string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.counts[userID]
}