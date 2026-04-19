package server

import "sync"

type Broker struct {
	mu      sync.RWMutex
	clients map[string]chan string // participantID → SSE channel
}

func NewBroker() *Broker {
	return &Broker{
		clients: make(map[string]chan string),
	}
}

func (b *Broker) Subscribe(participantID string) chan string {
	b.mu.Lock()
	defer b.mu.Unlock()

	if old, ok := b.clients[participantID]; ok {
		close(old)
	}

	ch := make(chan string, 16)
	b.clients[participantID] = ch
	return ch
}

// Unsubscribe removes the channel only if it matches the one currently registered.
// This prevents a reconnecting client's new channel from being closed by
// the old connection's deferred cleanup.
func (b *Broker) Unsubscribe(participantID string, ch chan string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if current, ok := b.clients[participantID]; ok && current == ch {
		close(current)
		delete(b.clients, participantID)
	}
}

func (b *Broker) IsConnected(participantID string) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	_, ok := b.clients[participantID]
	return ok
}

func (b *Broker) Send(participantID string, data string) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if ch, ok := b.clients[participantID]; ok {
		select {
		case ch <- data:
		default:
			// Drop if channel full — client is too slow
		}
	}
}
