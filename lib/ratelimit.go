package lib

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

var ErrRateLimited = errors.New("rate limit exceeded")
var ErrTooManyConnections = errors.New("too many connections")

type RateLimiter struct {
	rate       float64
	burst      int
	tokens     float64
	lastUpdate time.Time
	mu         sync.Mutex
	cond       *sync.Cond
}

func NewRateLimiter(rate float64, burst int) *RateLimiter {
	rl := &RateLimiter{
		rate:       rate,
		burst:      burst,
		tokens:     float64(burst),
		lastUpdate: time.Now(),
	}
	rl.cond = sync.NewCond(&rl.mu)
	return rl
}

func (rl *RateLimiter) refill() {
	now := time.Now()
	elapsed := now.Sub(rl.lastUpdate).Seconds()
	rl.lastUpdate = now
	rl.tokens += elapsed * rl.rate
	if rl.tokens > float64(rl.burst) {
		rl.tokens = float64(rl.burst)
	}
}

func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.refill()

	if rl.tokens >= 1 {
		rl.tokens--
		return true
	}

	return false
}

func (rl *RateLimiter) AllowN(n int) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.refill()

	if rl.tokens >= float64(n) {
		rl.tokens -= float64(n)
		return true
	}

	return false
}

func (rl *RateLimiter) Wait() error {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	for {
		rl.refill()
		if rl.tokens >= 1 {
			rl.tokens--
			return nil
		}
		rl.cond.Wait()
	}
}

func (rl *RateLimiter) WaitN(n int) error {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	for {
		rl.refill()
		if rl.tokens >= float64(n) {
			rl.tokens -= float64(n)
			return nil
		}
		rl.cond.Wait()
	}
}

func (rl *RateLimiter) Signal() {
	rl.cond.Signal()
}

func (rl *RateLimiter) Broadcast() {
	rl.cond.Broadcast()
}

type ConnectionLimiter struct {
	maxConns    int64
	activeConns int64
	rateLimiter *RateLimiter
	mu          sync.Mutex
}

func NewConnectionLimiter(maxConns int, rate float64, burst int) *ConnectionLimiter {
	return &ConnectionLimiter{
		maxConns:    int64(maxConns),
		activeConns: 0,
		rateLimiter: NewRateLimiter(rate, burst),
	}
}

func (cl *ConnectionLimiter) Acquire() error {
	if !cl.rateLimiter.Allow() {
		return ErrRateLimited
	}

	// Retry CAS up to 3 times to avoid false rejections under contention
	for i := 0; i < 3; i++ {
		current := atomic.LoadInt64(&cl.activeConns)
		if current >= cl.maxConns {
			return ErrTooManyConnections
		}
		if atomic.CompareAndSwapInt64(&cl.activeConns, current, current+1) {
			return nil
		}
	}
	return ErrTooManyConnections
}

func (cl *ConnectionLimiter) AcquireWithTimeout(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := cl.Acquire(); err != nil {
			time.Sleep(time.Millisecond)
			continue
		}
		return nil
	}
	return ErrTooManyConnections
}

func (cl *ConnectionLimiter) Release() {
	atomic.AddInt64(&cl.activeConns, -1)
}

func (cl *ConnectionLimiter) Active() int64 {
	return atomic.LoadInt64(&cl.activeConns)
}

func (cl *ConnectionLimiter) MaxConns() int64 {
	return cl.maxConns
}

func (cl *ConnectionLimiter) Available() int64 {
	return cl.maxConns - atomic.LoadInt64(&cl.activeConns)
}

type limiterItem struct {
	name  string
	limFn func() error
}

type MultiLimiter struct {
	limiters []limiterItem
	mu       sync.Mutex
}

func NewMultiLimiter() *MultiLimiter {
	return &MultiLimiter{
		limiters: make([]limiterItem, 0),
	}
}

func (ml *MultiLimiter) Add(name string, limFn func() error) {
	ml.limiters = append(ml.limiters, limiterItem{name: name, limFn: limFn})
}

func (ml *MultiLimiter) Allow() error {
	ml.mu.Lock()
	defer ml.mu.Unlock()

	for _, l := range ml.limiters {
		if err := l.limFn(); err != nil {
			for _, lb := range ml.limiters[:len(ml.limiters)-1] {
				lb.limFn()
			}
			return err
		}
	}
	return nil
}

type IPLimiter struct {
	connections map[string]int64
	maxPerIP    int64
	mu          sync.RWMutex
}

func NewIPLimiter(maxPerIP int64) *IPLimiter {
	return &IPLimiter{
		connections: make(map[string]int64),
		maxPerIP:    maxPerIP,
	}
}

func (il *IPLimiter) Allow(ip string) bool {
	il.mu.Lock()
	defer il.mu.Unlock()

	count, exists := il.connections[ip]
	if exists && count >= il.maxPerIP {
		return false
	}

	il.connections[ip] = count + 1
	return true
}

func (il *IPLimiter) Release(ip string) {
	il.mu.Lock()
	defer il.mu.Unlock()

	if count, ok := il.connections[ip]; ok {
		if count > 0 {
			il.connections[ip] = count - 1
		}
	}
}

func (il *IPLimiter) Count(ip string) int64 {
	il.mu.RLock()
	defer il.mu.RUnlock()
	return il.connections[ip]
}

func (il *IPLimiter) Clear() {
	il.mu.Lock()
	defer il.mu.Unlock()
	il.connections = make(map[string]int64)
}

type TokenBucket struct {
	tokens     float64
	capacity   float64
	refillRate float64
	lastRefill time.Time
	mu         sync.Mutex
	cond       *sync.Cond
}

func NewTokenBucket(capacity float64, refillRate float64) *TokenBucket {
	tb := &TokenBucket{
		tokens:     capacity,
		capacity:   capacity,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
	tb.cond = sync.NewCond(&tb.mu)
	return tb
}

func (tb *TokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.lastRefill = now
	tb.tokens += elapsed * tb.refillRate
	if tb.tokens > tb.capacity {
		tb.tokens = tb.capacity
	}
}

func (tb *TokenBucket) Take(tokens float64) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()

	if tb.tokens >= tokens {
		tb.tokens -= tokens
		return true
	}

	return false
}

func (tb *TokenBucket) WaitFor(tokens float64) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	for {
		tb.refill()
		if tb.tokens >= tokens {
			tb.tokens -= tokens
			return true
		}
		tb.cond.Wait()
	}
}
