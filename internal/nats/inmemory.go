package nats

import (
	"context"

	"github.com/calionauta/gogogo-fullstack-template/internal/queue"
)

// InMemoryBroadcaster fans out todo updates via the SSE Hub's Broadcast
// method. Suitable for single-instance deployments; for multi-instance
// or multi-user collaboration build with `-tags jetstream`, which swaps
// in the JetStream-backed broadcaster. Shared by both builds because the
// JetStream variant falls back to it on setup errors.
type InMemoryBroadcaster struct {
	hub *queue.SSEHub
}

// NewInMemoryBroadcaster returns a broadcaster bound to hub.
func NewInMemoryBroadcaster(hub *queue.SSEHub) *InMemoryBroadcaster {
	return &InMemoryBroadcaster{hub: hub}
}

// PublishTodoUpdate sends payload to every client registered on the hub.
func (b *InMemoryBroadcaster) PublishTodoUpdate(_ context.Context, payload []byte) error {
	if b.hub != nil {
		b.hub.Broadcast(payload)
	}
	return nil
}

// PublishTodoUpdateFrom sends payload to every client EXCEPT fromClientID.
// The originator already patched its own DOM via the per-request SSE
// response, so re-broadcasting to it would clobber the local view
// (e.g. replace its freshly-patched list with a full-list re-render that
// wipes rows). See SSEHub.BroadcastExcept.
func (b *InMemoryBroadcaster) PublishTodoUpdateFrom(_ context.Context, payload []byte, fromClientID string) error {
	if b.hub != nil {
		b.hub.BroadcastExcept(payload, fromClientID)
	}
	return nil
}

// Subscribe is a no-op for the in-memory broadcaster: it already holds
// the SSE Hub directly and fans out through it, so there is no separate
// transport to bind. Declared so callers can call Subscribe uniformly
// across both implementations.
func (b *InMemoryBroadcaster) Subscribe(_ *queue.SSEHub) {}
