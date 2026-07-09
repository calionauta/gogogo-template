//go:build turbine

package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/YakirOren/turbine"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
	sdk "github.com/starfederation/datastar-go/datastar"

	"github.com/calionauta/gogogo-fullstack-template/features/auth"
	"github.com/calionauta/gogogo-fullstack-template/internal/datastar"
	"github.com/calionauta/gogogo-fullstack-template/internal/nats"
	"github.com/calionauta/gogogo-fullstack-template/internal/queue"
	"github.com/calionauta/gogogo-fullstack-template/internal/workflow"
)

// PocketBaseTodoCreator implements workflow.TodoCreator by writing to the
// main app's "todos" collection. Registered as the package-level creator
// in RegisterOnboardingRoutes so the WelcomeOnboarding workflow can reach
// the main app DB from its durable steps.
type PocketBaseTodoCreator struct {
	app *pocketbase.PocketBase
	// broadcaster lets the durable workflow notify realtime clients
	// as each example todo is created, so the list grows live on every
	// connected screen instead of only after a manual refresh.
	broadcaster nats.TodoBroadcaster
}

// CreateExampleTodo inserts a new todo with the given title into the main
// app's "todos" collection and returns its PocketBase-generated id.
func (c *PocketBaseTodoCreator) CreateExampleTodo(ctx context.Context, title, user string) (string, error) {
	col, err := c.app.FindCollectionByNameOrId("todos")
	if err != nil {
		return "", fmt.Errorf("find todos collection: %w", err)
	}
	rec := core.NewRecord(col)
	rec.Set("title", title)
	rec.Set("completed", false)
	rec.Set("owner", user)
	if err := c.app.Save(rec); err != nil {
		return "", fmt.Errorf("save todo: %w", err)
	}
	// Notify all realtime clients that a new todo was created so the
	// list updates live as the durable workflow progresses.
	if c.broadcaster != nil {
		if err := c.broadcaster.PublishTodoUpdate(ctx, todoUpdateJob("created", rec.Id, title, false)); err != nil {
			slog.Warn("onboarding: broadcast todo created failed", "error", err)
		}
		// Advance the live stepper: this example todo is step (n+1) of 5.
		step := nextOnboardingCreateStep(user)
		publishOnboardingProgress(c.broadcaster, ctx, step+1, 5, "create_todo",
			fmt.Sprintf("Step %d/5 — Created example todo: %s", step+1, title))
	}
	return rec.Id, nil
}

// onboardingStepCounts tracks how many example todos a user's workflow
// has created, so we can number the create steps (2..4 of 5) as the
// durable workflow advances. Keyed by user; reset when a new workflow
// starts (handleStart calls resetOnboardingSteps).
var onboardingStepCounts sync.Map // user -> *atomic.Int32

func resetOnboardingSteps(user string) {
	onboardingStepCounts.Store(user, &atomic.Int32{})
}

func nextOnboardingCreateStep(user string) int {
	v, _ := onboardingStepCounts.LoadOrStore(user, &atomic.Int32{})
	return int(v.(*atomic.Int32).Add(1))
}

// publishOnboardingProgress streams a "progress" event through the
// broadcaster so every connected client sees the durable workflow
// advance step by step — Turbine's durability and JetStream's realtime
// delivery, observed together in the UI stepper.
func publishOnboardingProgress(b nats.TodoBroadcaster, ctx context.Context, step, total int, phase, detail string) {
	if b == nil {
		return
	}
	p := mustJSON(map[string]any{
		"step":   step,
		"total":  total,
		"phase":  phase,
		"detail": detail,
	})
	envelope := mustJSON(queue.Job{Type: "progress", Payload: p})
	if err := b.PublishTodoUpdate(ctx, envelope); err != nil {
		slog.Warn("onboarding: publish progress failed", "error", err)
	}
}

// RegisterOnboardingRoutes wires the onboarding HTTP routes into the
// router and registers the PocketBase-backed TodoCreator so the workflow
// can write to the main app DB. Build-tag gated: only compiled when the
// binary is built with `-tags turbine`.
func RegisterOnboardingRoutes(
	app *pocketbase.PocketBase,
	q *queue.Queue,
	rt *workflow.Runtime,
	r *router.Router[*core.RequestEvent],
	broadcaster nats.TodoBroadcaster,
) {
	workflow.RegisterTodoCreator(&PocketBaseTodoCreator{app: app, broadcaster: broadcaster})

	if r != nil && rt != nil {
		h := &OnboardingHandler{app: app, q: q, rt: rt, broadcaster: broadcaster}
		// LoadAppAuth populates c.Auth from the demo user's gogogo_auth
		// cookie (the global LoadAuthFromCookie skips /api/), so the
		// workflow scopes the created todos to the logged-in user's
		// tenant instead of falling back to an anonymous "friend".
		r.POST("/api/onboarding/start", h.handleStart).BindFunc(auth.LoadAppAuth)
	}
}

// OnboardingHandler exposes the WelcomeOnboarding workflow over HTTP.
// The workflow runs asynchronously inside Turbine, so the HTTP response
// is just a "started" toast + a "started" signal. The todo list will
// grow as each step completes (each step writes a todo via TodoCreator).
type OnboardingHandler struct {
	app *pocketbase.PocketBase
	q   *queue.Queue
	rt  *workflow.Runtime
	// broadcaster is used to refresh every client's list once the
	// workflow finishes.
	broadcaster nats.TodoBroadcaster
}

func (h *OnboardingHandler) handleStart(c *core.RequestEvent) error {
	if err := c.Request.ParseForm(); err != nil {
		return c.String(http.StatusBadRequest, "invalid form")
	}
	user := c.Request.FormValue("user")
	if user == "" && c.Auth != nil {
		// Default to the authenticated user who clicked the button so the
		// example todos are scoped to their tenant and actually appear
		// in their list. The "user" form value (e.g. a scanned
		// profile) still wins when present.
		user = c.Auth.Id
	}
	if user == "" {
		user = "friend"
	}

	// Reset the per-user step counter and announce step 1 so the UI
	// stepper lights up the moment the workflow starts.
	resetOnboardingSteps(user)
	publishOnboardingProgress(h.broadcaster, context.Background(), 1, 5, "greet", "Step 1/5 — Greeting user")

	// Fire the workflow in a goroutine. Turbine persists state in SQLite
	// after each step, so a crash mid-run resumes at the last completed
	// step — no duplicate todos.
	go func() {
		handle, err := turbine.Run(h.rt.T(), workflow.WelcomeOnboarding, user)
		if err != nil {
			slog.Error("onboarding: workflow start failed", "user", user, "error", err)
			return
		}
		// Poll for completion to log the outcome. The actual progress is
		// visible in the todo list as each step's side effect lands.
		for {
			result, err := handle.GetResult()
			if err == nil {
				slog.Info("onboarding: workflow completed",
					"user", user, "todos", len(result))
				// Step 5/5 + a final list refresh so every connected
				// client sees the completed durable workflow.
				publishOnboardingProgress(h.broadcaster, context.Background(), 5, 5, "finalize", "Step 5/5 — Onboarding complete")
				if h.broadcaster != nil {
					if err := h.broadcaster.PublishTodoUpdate(
						context.Background(),
						todoUpdateJob("workflow-completed", "", "", false),
					); err != nil {
						slog.Warn("onboarding: broadcast workflow-completed failed", "error", err)
					}
				}
				return
			}
			time.Sleep(200 * time.Millisecond)
		}
	}()

	sse := sdk.NewSSE(c.Response, c.Request)
	if err := datastar.MergeSignals(sse, map[string]any{
		"onboardingStarted": true,
		"onboardingUser":    user,
	}); err != nil {
		return err
	}
	return emitToast(sse, "Onboarding started — watch the list grow", "info")
}
