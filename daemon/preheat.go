package daemon

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"
)

// Preheater maintains warm connections and pre-initialized state
// to eliminate cold-start latency on first request.
type Preheater struct {
	mu        sync.Mutex
	transport *http.Transport
	warmedAt  time.Time
	interval  time.Duration
	cancel    context.CancelFunc
	ready     bool
}

// NewPreheater creates a preheater with the given warmup interval.
func NewPreheater(interval time.Duration) *Preheater {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &Preheater{
		interval: interval,
		transport: &http.Transport{
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 5,
			IdleConnTimeout:     90 * time.Second,
			DialContext: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
		},
	}
}

// Start begins background warmup. Call Stop() to clean up.
func (p *Preheater) Start(endpoints []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cancel != nil {
		return // already running
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.ready = true

	go p.warmLoop(ctx, endpoints)
}

// Stop terminates the background warmup goroutine.
func (p *Preheater) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	p.ready = false
}

// Ready reports whether the preheater has completed at least one warmup cycle.
func (p *Preheater) Ready() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.ready && !p.warmedAt.IsZero()
}

// Transport returns the pre-warmed HTTP transport for reuse.
func (p *Preheater) Transport() *http.Transport {
	return p.transport
}

// warmLoop periodically pings endpoints to keep connections alive.
func (p *Preheater) warmLoop(ctx context.Context, endpoints []string) {
	// Immediate first warmup
	p.warmOnce(ctx, endpoints)

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.warmOnce(ctx, endpoints)
		}
	}
}

func (p *Preheater) warmOnce(ctx context.Context, endpoints []string) {
	client := &http.Client{
		Transport: p.transport,
		Timeout:   5 * time.Second,
	}
	for _, ep := range endpoints {
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, ep, nil)
		if err != nil {
			continue
		}
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
		}
	}
	p.mu.Lock()
	p.warmedAt = time.Now()
	p.mu.Unlock()
}
