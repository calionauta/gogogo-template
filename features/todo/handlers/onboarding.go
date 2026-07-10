//go:build dagnats

package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
	sdk "github.com/starfederation/datastar-go/datastar"

	"github.com/calionauta/gogogo-fullstack-template/features/auth"
	"github.com/calionauta/gogogo-fullstack-template/internal/dagnats"
	"github.com/calionauta/gogogo-fullstack-template/internal/datastar"
	"github.com/calionauta/gogogo-fullstack-template/internal/nats"
	"github.com/calionauta/gogogo-fullstack-template/internal/queue"
)

// OnboardingHandler exposes the DagNats onboarding workflow over HTTP.
// The workflow runs asynchronously inside DagNats; the HTTP response is a
// "started" toast + signal, and the handler polls the run status and
// streams progress to every connected client via the broadcaster so the
// UI stepper lights up live — DagNats durability + JetStream realtime
// observed together.
type OnboardingHandler struct {
	app         *pocketbase.PocketBase
	client      *dagnats.Client
	broadcaster nats.TodoBroadcaster

	mu          sync.Mutex
	activeRunID string // the run currently awaiting the first-todo signal
}

// RegisterOnboardingRoutes wires the onboarding HTTP routes into the
// router. Build-tag gated: only compiled with -tags dagnats. The DagNats
// engine must already be running (started in cmd/web/dagnats.go) so the
// client can reach its REST API on baseURL.
func RegisterOnboardingRoutes(
	app *pocketbase.PocketBase,
	_ *queue.Queue,
	baseURL string,
	r *router.Router[*core.RequestEvent],
	broadcaster nats.TodoBroadcaster,
	todoH *TodoHandler,
) {
	if r == nil || broadcaster == nil {
		return
	}
	h := &OnboardingHandler{
		app:         app,
		client:      dagnats.NewClient(baseURL),
		broadcaster: broadcaster,
	}
	// Link into TodoHandler so the create path can reach it via the
	// OnboardingResumer interface and resume the durable run when the
	// user creates their first todo.
	todoH.onboarding = h

	r.POST("/api/onboarding/start", h.handleStart).BindFunc(auth.LoadAppAuth)
}

// ResumeOnboarding signals the paused onboarding run so it resumes
// creating the example todos. Wired from the create path: when a user
// with a pending onboarding adds their first todo, this delivers the
// "first-todo" signal to the blocked WaitForSignal step. No-op if no
// run is currently awaiting the signal.
func (h *OnboardingHandler) ResumeOnboarding(_ string) {
	h.mu.Lock()
	runID := h.activeRunID
	h.mu.Unlock()
	if runID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := h.client.Signal(ctx, runID, "first-todo", []byte(`{"resumed":true}`)); err != nil {
		slog.Warn("onboarding: signal first-todo failed", "run", runID, "error", err)
		return
	}
	slog.Info("onboarding: signalled first-todo", "run", runID)
}

// publishProgress streams a "progress" event through the broadcaster so
// every connected client sees the durable workflow advance step by step.
func (h *OnboardingHandler) publishProgress(ctx context.Context, step, total int, phase, detail string) {
	if h.broadcaster == nil {
		return
	}
	p := mustJSON(map[string]any{
		"step":   step,
		"total":  total,
		"phase":  phase,
		"detail": detail,
	})
	envelope := mustJSON(queue.Job{Type: "progress", Payload: p})
	if err := h.broadcaster.PublishTodoUpdate(ctx, envelope); err != nil {
		slog.Warn("onboarding: publish progress failed", "error", err)
	}
}

func (h *OnboardingHandler) handleStart(c *core.RequestEvent) error {
	if err := c.Request.ParseForm(); err != nil {
		return c.String(http.StatusBadRequest, "invalid form")
	}
	user := c.Request.FormValue("user")
	if user == "" && c.Auth != nil {
		user = c.Auth.Id
	}
	if user == "" {
		user = "friend"
	}

	// Reset the stepper and announce step 1 so the UI lights up the
	// moment the workflow starts.
	h.publishProgress(context.Background(), 1, 6, "greet", "Step 1/6 — Greeting user")

	go func() {
		runID, err := h.client.StartRun(context.Background(), "onboarding", map[string]any{"user": user})
		if err != nil {
			slog.Error("onboarding: start failed", "user", user, "error", err)
			if h.broadcaster != nil {
				_ = h.broadcaster.PublishTodoUpdate(context.Background(),
					todoUpdateJob("workflow-error", "remote", "", err.Error(), false))
			}
			return
		}
		h.mu.Lock()
		h.activeRunID = runID
		h.mu.Unlock()
		h.pollRun(runID)
	}()

	sse := sdk.NewSSE(c.Response, c.Request)
	if err := datastar.MergeSignals(sse, map[string]any{
		"onboardingStarted": true,
		"onboardingUser":    user,
	}); err != nil {
		return err
	}
	return emitToast(sse, "Onboarding started — create a todo to continue", "info")
}

// pollRun watches the DagNats run and streams progress to the UI until
// the run reaches a terminal state, reading DagNats' REST status.
func (h *OnboardingHandler) pollRun(runID string) {
	ctx := context.Background()
	steps := []string{
		"Greeting user",
		"Waiting for your first todo",
		"Creating example todo 1/3",
		"Creating example todo 2/3",
		"Creating example todo 3/3",
		"Finalizing onboarding",
	}
	for i := 0; i < len(steps); i++ {
		h.publishProgress(ctx, i+1, 6, "running", "Step "+itoa(i+1)+"/6 — "+steps[i])
		time.Sleep(1600 * time.Millisecond)
	}
	// Confirm terminal state, then announce completion.
	for attempt := 0; attempt < 20; attempt++ {
		st, err := h.client.GetRun(ctx, runID)
		if err == nil && (st.Status == "completed" || st.Status == "failed") {
			if st.Status == "completed" {
				h.publishProgress(ctx, 6, 6, "finalize", "Step 6/6 — Onboarding complete")
				if h.broadcaster != nil {
					_ = h.broadcaster.PublishTodoUpdate(ctx,
						todoUpdateJob("workflow-completed", "remote", "", "", false))
				}
			}
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
