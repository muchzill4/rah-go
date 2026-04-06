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

func (b *Broker) Unsubscribe(participantID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if ch, ok := b.clients[participantID]; ok {
		close(ch)
		delete(b.clients, participantID)
	}
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
