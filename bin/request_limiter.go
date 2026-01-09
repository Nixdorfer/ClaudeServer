package main

import (
	"sync"
	"time"
)

type RequestLimiter struct {
	lastRequestTime time.Time
	intervalMs      int
	mutex           sync.Mutex
}

var globalRequestLimiter *RequestLimiter

func InitRequestLimiter(intervalMs int) {
	globalRequestLimiter = &RequestLimiter{
		lastRequestTime: time.Time{},
		intervalMs:      intervalMs,
	}
}

func WaitForNextRequest() {
	if globalRequestLimiter == nil {
		return
	}

	globalRequestLimiter.mutex.Lock()
	defer globalRequestLimiter.mutex.Unlock()

	if globalRequestLimiter.lastRequestTime.IsZero() {
		globalRequestLimiter.lastRequestTime = time.Now()
		return
	}

	elapsed := time.Since(globalRequestLimiter.lastRequestTime)
	requiredInterval := time.Duration(globalRequestLimiter.intervalMs) * time.Millisecond

	if elapsed < requiredInterval {
		waitTime := requiredInterval - elapsed
		time.Sleep(waitTime)
	}

	globalRequestLimiter.lastRequestTime = time.Now()
}
