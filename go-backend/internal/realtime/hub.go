// Package realtime distributes monitoring events without blocking ingestion.
package realtime

import (
	"encoding/json"
	"sync"
	"time"
)

type Event struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Data      any       `json:"data"`
}

type Subscription struct {
	Messages chan []byte
	Done     chan struct{}
}

type Hub struct {
	mu      sync.Mutex
	closed  bool
	queue   int
	clients map[*Subscription]struct{}
}

func NewHub(queue int) *Hub {
	if queue < 1 {
		queue = 32
	}
	return &Hub{queue: queue, clients: make(map[*Subscription]struct{})}
}

func (h *Hub) Subscribe() *Subscription {
	h.mu.Lock()
	defer h.mu.Unlock()
	s := &Subscription{Messages: make(chan []byte, h.queue), Done: make(chan struct{})}
	if h.closed {
		close(s.Done)
	} else {
		h.clients[s] = struct{}{}
	}
	return s
}

func (h *Hub) Unsubscribe(s *Subscription) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.clients[s]; ok {
		delete(h.clients, s)
		close(s.Done)
	}
}

func (h *Hub) Publish(kind string, data any, now time.Time) {
	payload, err := json.Marshal(Event{Type: kind, Timestamp: now.UTC(), Data: data})
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.clients {
		select {
		case s.Messages <- payload:
		default:
			delete(h.clients, s)
			close(s.Done)
		}
	}
}

func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for s := range h.clients {
		delete(h.clients, s)
		close(s.Done)
	}
}
