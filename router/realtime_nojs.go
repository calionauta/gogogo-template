//go:build !jetstream

package router

import (
	"github.com/calionauta/cali-go-stack/internal/nats"
	"github.com/calionauta/cali-go-stack/internal/queue"
)

// newTodoBroadcaster builds the default in-memory broadcaster. Todo
// mutations are fanned out to every client connected to this process
// via the SSE Hub.
func newTodoBroadcaster(hub *queue.SSEHub) nats.TodoBroadcaster {
	return nats.NewTodoBroadcaster(nil, hub)
}
