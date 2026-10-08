package realtime

import (
	"testing"
	"time"
)

func TestSlowClientDoesNotBlockBroadcast(t *testing.T) {
	h := NewHub(1)
	defer h.Close()
	slow, fast := h.Subscribe(), h.Subscribe()
	for i := 0; i < 100; i++ {
		h.Publish("telemetry", map[string]int{"seq": i}, time.Now())
		select {
		case <-fast.Messages:
		case <-time.After(time.Second):
			t.Fatal("fast client blocked")
		}
	}
	select {
	case <-slow.Done:
	default:
		t.Fatal("slow client not evicted")
	}
	select {
	case <-fast.Done:
		t.Fatal("healthy client evicted")
	default:
	}
	h.Unsubscribe(slow)
	h.Unsubscribe(fast)
	h.Unsubscribe(fast)
	h.Close()
	closed := h.Subscribe()
	select {
	case <-closed.Done:
	default:
		t.Fatal("closed hub accepted subscription")
	}
}
