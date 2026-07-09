//go:build !turbine

package router

import (
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"github.com/calionauta/gogogo-fullstack-template/features/todo/handlers"
	"github.com/calionauta/gogogo-fullstack-template/internal/nats"
	"github.com/calionauta/gogogo-fullstack-template/internal/queue"
)

// registerOnboarding is a no-op when Turbine is not enabled. The router
// stays importable from default builds; the onboarding routes simply
// don't exist. The broadcaster argument is accepted for signature
// parity with the turbine build but ignored here.
func registerOnboarding(
	_ *pocketbase.PocketBase,
	_ *queue.Queue,
	_ *core.ServeEvent,
	_ WorkflowRuntime,
	_ nats.TodoBroadcaster,
	_ *handlers.TodoHandler,
) {
	// Turbine not available without -tags turbine
}
