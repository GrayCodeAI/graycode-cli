package alerts

import (
	"sync"
	"time"
)

type Alert struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Entity    string    `json:"entity"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
	Delivered bool      `json:"delivered"`
}

type CooldownConfig struct {
	Period        time.Duration `json:"period"`
	MaxPending    int           `json:"max_pending"`
	DrainInterval time.Duration `json:"drain_interval"`
	SendDelay     time.Duration `json:"send_delay"`
}

func DefaultCooldownConfig() CooldownConfig {
	return CooldownConfig{
		Period:        24 * time.Hour,
		MaxPending:    100,
		DrainInterval: 5 * time.Minute,
		SendDelay:     time.Second,
	}
}

type AlertQueue struct {
	mu        sync.Mutex
	pending   []*Alert
	cooldowns map[string]time.Time // entity+type -> last sent
	config    CooldownConfig
	handler   func(*Alert) error
	stopCh    chan struct{}
}

func NewAlertQueue(config CooldownConfig, handler func(*Alert) error) *AlertQueue {
	return &AlertQueue{
		pending:   make([]*Alert, 0),
		cooldowns: make(map[string]time.Time),
		config:    config,
		handler:   handler,
		stopCh:    make(chan struct{}),
	}
}

func (q *AlertQueue) Enqueue(alert *Alert) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	key := alert.Entity + ":" + alert.Type
	if last, ok := q.cooldowns[key]; ok && time.Since(last) < q.config.Period {
		return false
	}

	if len(q.pending) >= q.config.MaxPending {
		return false
	}

	alert.CreatedAt = time.Now()
	q.pending = append(q.pending, alert)
	return true
}

func (q *AlertQueue) Start() {
	go q.drainLoop()
}

func (q *AlertQueue) Stop() {
	close(q.stopCh)
}

func (q *AlertQueue) drainLoop() {
	ticker := time.NewTicker(q.config.DrainInterval)
	defer ticker.Stop()

	for {
		select {
		case <-q.stopCh:
			return
		case <-ticker.C:
			q.drain()
		}
	}
}

func (q *AlertQueue) drain() {
	q.mu.Lock()
	batch := q.pending
	q.pending = make([]*Alert, 0)
	q.mu.Unlock()

	for _, alert := range batch {
		if q.handler != nil {
			if err := q.handler(alert); err == nil {
				alert.Delivered = true
				q.mu.Lock()
				q.cooldowns[alert.Entity+":"+alert.Type] = time.Now()
				q.mu.Unlock()
			}
		}
		time.Sleep(q.config.SendDelay)
	}
}

func (q *AlertQueue) PendingCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.pending)
}
