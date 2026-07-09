//go:build turbine

package router

import (
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"github.com/calionauta/gogogo-fullstack-template/features/todo/handlers"
	"github.com/calionauta/gogogo-fullstack-template/internal/nats"
	"github.com/calionauta/gogogo-fullstack-template/internal/queue"
	"github.com/calionauta/gogogo-fullstack-template/internal/workflow"
)

// registerOnboarding wires the WelcomeOnboarding workflow into the
// PocketBase router. Called from Init when Turbine is enabled.
func registerOnboarding(
	app *pocketbase.PocketBase,
	q *queue.Queue,
	se *core.ServeEvent,
	rt WorkflowRuntime,
	broadcaster nats.TodoBroadcaster,
	todoH *handlers.TodoHandler,
) {
	if rt == nil || todoH == nil {
		return
	}
	concrete, ok := rt.(*workflow.Runtime)
	if !ok {
		return
	}
	handlers.RegisterOnboardingRoutes(app, q, concrete, se.Router, broadcaster, todoH)
}
