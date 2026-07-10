//go:build !dagnats

package router

import (
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"github.com/calionauta/gogogo-fullstack-template/features/todo/handlers"
	"github.com/calionauta/gogogo-fullstack-template/internal/nats"
	"github.com/calionauta/gogogo-fullstack-template/internal/queue"
)

// registerOnboarding is a no-op without -tags dagnats. The router stays
// importable from default builds; the onboarding routes simply don't
// exist. The broadcaster argument is accepted for signature parity with
// the dagnats build but ignored here.
func registerOnboarding(
	_ *pocketbase.PocketBase,
	_ *queue.Queue,
	_ *core.ServeEvent,
	_ nats.TodoBroadcaster,
	_ *handlers.TodoHandler,
	_ string,
) {
	// DagNats not available without -tags dagnats
}
