// Package eventbus provides an in-process publish/subscribe broker used to
// bridge events produced by CLI commands to SSE streams served to the
// browser UI.
package eventbus

import "sync"

// Event is a single message broadcast to subscribers.
type Event struct {
	// Type is the SSE event name (e.g. "heartbeat").
	Type string
	// Data is the SSE payload, already serialized (typically JSON or plain text).
	Data string
}

// subscriberBuffer is how many pending events a slow subscriber can queue
// before new events are dropped for it.
const subscriberBuffer = 16

// Bus broadcasts events to any number of subscribers. The zero value is not
// usable; construct one with New.
type Bus struct {
	mu   sync.Mutex
	subs map[chan Event]struct{}
}

// New creates a ready-to-use Bus.
func New() *Bus {
	return &Bus{
		subs: make(map[chan Event]struct{}),
	}
}

// Subscribe registers a new listener and returns a channel of events along
// with an unsubscribe function. The caller must call unsubscribe when done
// listening to avoid leaking the channel.
func (b *Bus) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, subscriberBuffer)

	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()

	unsubscribe := func() {
		b.mu.Lock()
		if _, ok := b.subs[ch]; ok {
			delete(b.subs, ch)
			close(ch)
		}
		b.mu.Unlock()
	}

	return ch, unsubscribe
}

// Publish broadcasts an event to all current subscribers. Subscribers whose
// buffer is full are skipped rather than blocking the publisher.
func (b *Bus) Publish(e Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for ch := range b.subs {
		select {
		case ch <- e:
		default:
		}
	}
}
