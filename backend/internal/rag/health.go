package rag

import (
	"context"
	"log"
	"sync/atomic"
	"time"
)

// HealthMonitor polls the RAG service /health endpoint in the background
// and exposes a lock-free IsHealthy() check via atomic.Bool.
type HealthMonitor struct {
	client   *Client
	healthy  atomic.Bool
	interval time.Duration
}

// NewHealthMonitor creates a health monitor. Call Start() to begin polling.
func NewHealthMonitor(client *Client, interval time.Duration) *HealthMonitor {
	return &HealthMonitor{
		client:   client,
		interval: interval,
	}
}

// Start begins the background health polling loop. It runs until ctx is cancelled.
func (h *HealthMonitor) Start(ctx context.Context) {
	// Do an immediate check before starting the ticker.
	h.check()

	go func() {
		ticker := time.NewTicker(h.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				h.check()
			}
		}
	}()
}

// IsHealthy returns the last known health status. Lock-free.
func (h *HealthMonitor) IsHealthy() bool {
	return h.healthy.Load()
}

func (h *HealthMonitor) check() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	resp, err := h.client.CheckHealth(ctx)
	wasHealthy := h.healthy.Load()

	if err != nil || resp == nil {
		h.healthy.Store(false)
		if wasHealthy {
			log.Printf("RAG health check failed: %v", err)
		}
		return
	}

	nowHealthy := resp.Status == "healthy" || resp.Status == "demo"
	h.healthy.Store(nowHealthy)

	if nowHealthy && !wasHealthy {
		log.Printf("RAG service connected (status: %s)", resp.Status)
	} else if !nowHealthy && wasHealthy {
		log.Printf("RAG service degraded (status: %s)", resp.Status)
	}
}
