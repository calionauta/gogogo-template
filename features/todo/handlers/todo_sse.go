package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/a-h/templ"
	"github.com/google/uuid"
	"github.com/pocketbase/pocketbase/core"
	sdk "github.com/starfederation/datastar-go/datastar"

	"github.com/calionauta/gogogo-fullstack-template/features/todo"
	"github.com/calionauta/gogogo-fullstack-template/features/todo/components"
	dshelpers "github.com/calionauta/gogogo-fullstack-template/internal/datastar"
	"github.com/calionauta/gogogo-fullstack-template/internal/queue"
)

func (h *TodoHandler) handleSSEStream(c *core.RequestEvent) error {
	clientID := c.Request.URL.Query().Get("clientID")
	if clientID == "" {
		clientID = uuid.New().String()
	}

	sse := sdk.NewSSE(c.Response, c.Request)
	ch := make(chan []byte, sseClientBuffer)
	h.q.Hub().Register(clientID, ch)
	defer func() {
		h.q.Hub().Unregister(clientID)
		h.broadcastClientCount()
	}()

	todos, err := h.listTodos(c, "all")
	if err != nil {
		slog.Error("todo: list on sse open failed", "error", err)
		return c.String(statusInternal, "error listing todos")
	}
	if err := dshelpers.MergeSignals(sse, todo.Signals{
		Todos:            todos,
		Filter:           "all",
		ItemCount:        len(todos),
		AdminEnabled:     h.cfg.AdminToken != "",
		LLMEnabled:       h.llmEnabled(),
		SimulatedLLM:     h.simulatedLLMEnabled(),
		WorkflowEnabled:  h.cfg.Workflow.Enabled,
		ConnectedClients: h.q.Hub().Stats().Clients,
		ClientID:         clientID,
	}); err != nil {
		return err
	}
	// Tell every connected client how many are online now.
	h.broadcastClientCount()

	for {
		select {
		case <-c.Request.Context().Done():
			return nil
		case msg := <-ch:
			if err := h.dispatchStreamMessage(c, sse, msg); err != nil {
				slog.Warn("todo: sse dispatch failed", "error", err)
			}
		}
	}
}

// dispatchStreamMessage routes a queued SSE chunk to the right client-side
// patch. Worker output arrives as a queue.Job envelope: type "toast" is
// rendered as a Templ component, type "retry" is merged as a signal so
// the user sees per-attempt feedback, anything else is forwarded as a
// raw signal (defensive default).
func (h *TodoHandler) dispatchStreamMessage(c *core.RequestEvent, sse *sdk.ServerSentEventGenerator, msg []byte) error {
	var job queue.Job
	if err := json.Unmarshal(msg, &job); err != nil {
		return fmt.Errorf("decode stream envelope: %w", err)
	}
	switch job.Type {
	case jobTypeToast:
		var p struct {
			ToastType string `json:"toastType"`
			Message   string `json:"message"`
		}
		if err := json.Unmarshal(job.Payload, &p); err != nil {
			return fmt.Errorf("decode toast payload: %w", err)
		}
		if p.ToastType == "" {
			p.ToastType = "info"
		}
		return emitToast(sse, p.Message, p.ToastType)
	case "retry":
		// Retry feedback from the queue worker. Merge the raw payload
		// (so the UI can read operation/attempt/status) and emit a
		// friendly narration toast so queued work is observable end to
		// end - especially the Suggest async lifecycle.
		var p struct {
			Operation string `json:"operation"`
			Attempt   int    `json:"attempt"`
			Status    string `json:"status"`
			Error     string `json:"error"`
		}
		if err := json.Unmarshal(job.Payload, &p); err != nil {
			return fmt.Errorf("decode retry payload: %w", err)
		}
		if err := dshelpers.MergeSignals(sse, map[string]any{
			"lastRetry": string(job.Payload),
		}); err != nil {
			return err
		}
		verb := p.Operation
		if verb == "suggest_simulated" {
			verb = "suggest (simulated)"
		}
		var msg, kind string
		switch p.Status {
		case "success":
			msg, kind = fmt.Sprintf("%s: completed", verb), "success"
		default: // "attempt"
			msg = fmt.Sprintf("%s: attempt %d failed", verb, p.Attempt)
			if p.Error != "" {
				msg += " (" + p.Error + ")"
			}
			msg += ", retrying…"
			kind = "warning"
		}
		return emitToast(sse, msg, kind)
	case "todo":
		// A todo was created/updated/deleted (possibly by another
		// client or the durable workflow). Re-render the list for
		// THIS client so every screen stays in sync in real time.
		//
		// The durable workflow also emits a synthetic todo update of
		// type "workflow-completed" once all five steps finish. That
		// acts as the completion signal so the UI can show a final
		// alert (the stepper reaching 5/5 + the list growing live).
		var tp struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(job.Payload, &tp); err == nil && tp.Type == "workflow-completed" {
			if err := dshelpers.MergeSignals(sse, map[string]any{
				"onboardingActive":  true,
				"onboardingPhase":   "completed",
				"workflowCompleted": true,
			}); err != nil {
				return err
			}
			return emitToast(sse, "Workflow completed — 3 example todos created", "success")
		}
		todos, err := h.listTodos(c, "all")
		if err != nil {
			return fmt.Errorf("list todos for broadcast: %w", err)
		}
		return dshelpers.RenderAndPatch(sse, h.renderTodoList(todos), sdk.WithSelector("#todo-list"))
	case "clients":
		var p struct {
			Count int `json:"count"`
		}
		if err := json.Unmarshal(job.Payload, &p); err != nil {
			return fmt.Errorf("decode clients payload: %w", err)
		}
		return dshelpers.MergeSignals(sse, map[string]any{
			"connectedClients": p.Count,
		})
	case "suggest_result":
		// AI suggest result from the queue worker: merge the suggestions
		// (or error) into signals and narrate the outcome with a toast.
		var p struct {
			Suggestions    []string `json:"suggestions"`
			SuggestErr     string   `json:"suggestErr"`
			SuggestPending bool     `json:"suggestPending"`
		}
		if err := json.Unmarshal(job.Payload, &p); err != nil {
			return fmt.Errorf("decode suggest_result payload: %w", err)
		}
		if err := dshelpers.MergeSignals(sse, map[string]any{
			"suggestions":    p.Suggestions,
			"suggestErr":     p.SuggestErr,
			"suggestPending": p.SuggestPending,
		}); err != nil {
			return err
		}
		if p.SuggestErr != "" {
			return emitToast(sse, "Suggest failed: "+p.SuggestErr, "error")
		}
		return emitToast(sse, fmt.Sprintf("Got %d suggestions", len(p.Suggestions)), "success")

	case "progress":
		// Durable-workflow progress (Turbine). Streamed as each step
		// completes so the UI can render a live stepper and the user
		// can watch the workflow advance — and, because the steps are
		// durable, see it resume after a restart. Merged as signals and
		// echoed as a toast so the progression is both visible and
		// narrated.
		var p struct {
			Step   int    `json:"step"`
			Total  int    `json:"total"`
			Phase  string `json:"phase"`
			Detail string `json:"detail"`
		}
		if err := json.Unmarshal(job.Payload, &p); err != nil {
			return fmt.Errorf("decode progress payload: %w", err)
		}
		if err := dshelpers.MergeSignals(sse, map[string]any{
			"onboardingActive": true,
			"onboardingStep":   p.Step,
			"onboardingTotal":  p.Total,
			"onboardingPhase":  p.Phase,
			"onboardingDetail": p.Detail,
		}); err != nil {
			return err
		}
		return emitToast(sse, p.Detail, "info")
	default:
		return dshelpers.MergeSignals(sse, map[string]any{
			"lastJob": string(msg),
		})
	}
}

// broadcastClientCount tells every connected client how many are
// currently online. Called on connect + disconnect so the UI's
// presence badge stays live.
func (h *TodoHandler) broadcastClientCount() {
	payload := mustJSON(map[string]any{"count": h.q.Hub().Stats().Clients})
	h.q.Hub().Broadcast(mustJSON(queue.Job{Type: "clients", Payload: payload}))
}

// on the client. The toast's open state, dismiss timer, and progress bar
// are all driven by Datastar attributes on the rendered template.
func emitToast(sse *sdk.ServerSentEventGenerator, message, toastType string) error {
	return dshelpers.RenderAndPatch(
		sse,
		components.Toast(message, toastType, components.NewToastID()),
		sdk.WithSelectorID("toast-container"),
		sdk.WithModeAppend(),
	)
}

// --- Repository ---

// listTodos returns the authenticated user's todos, scoped by the
// todos.owner relation set on create. When the request is
// unauthenticated the filter is left unscoped — the single-tenant demo
// fallback; every production route requires login via RequireAuth.
func (h *TodoHandler) listTodos(c *core.RequestEvent, filter string) ([]todo.Todo, error) {
	var filterExpr string
	switch filter {
	case "active":
		filterExpr = "completed=false"
	case "completed":
		filterExpr = "completed=true"
	default:
		filterExpr = ""
	}
	if c != nil && c.Auth != nil {
		ownerFilter := fmt.Sprintf("owner = %q", c.Auth.Id)
		if filterExpr == "" {
			filterExpr = ownerFilter
		} else {
			filterExpr = filterExpr + " && " + ownerFilter
		}
	}
	records, err := h.app.FindRecordsByFilter("todos", filterExpr, "-created", 0, 0)
	if err != nil {
		return nil, fmt.Errorf("find todos (filter=%q): %w", filter, err)
	}
	res := make([]todo.Todo, len(records))
	for i, r := range records {
		res[i] = todoFromRecord(r)
	}
	return res, nil
}

// clearCompletedFilter builds the FindRecordsByFilter expression for the
// "clear completed" action, scoping it to the authenticated user.
func clearCompletedFilter(c *core.RequestEvent) string {
	if c != nil && c.Auth != nil {
		return fmt.Sprintf("completed=true && owner = %q", c.Auth.Id)
	}
	return "completed=true"
}

func (h *TodoHandler) saveTodo(item *todo.Todo, owner string) error {
	col, err := h.app.FindCollectionByNameOrId("todos")
	if err != nil {
		return fmt.Errorf("find todos collection: %w", err)
	}
	rec := core.NewRecord(col)
	// PocketBase auto-generates a 15-char id when none is set on the
	// record. Don't pass a client-side uuid here — the collection's
	// primary key has Max=15 enforced by PocketBase.
	rec.Set("title", item.Title)
	rec.Set("completed", item.Completed)
	// Scope the todo to the authenticated user when available so todos
	// are tenant-associated (the demo user sees only their own). The
	// owner field is added by db.SeedDefaults; missing-auth creates are
	// left unscoped (single-tenant demo fallback).
	if owner != "" {
		rec.Set("owner", owner)
	}
	if err := h.app.Save(rec); err != nil {
		return fmt.Errorf("save todo: %w", err)
	}
	item.ID = rec.Id
	return nil
}

func todoFromRecord(r *core.Record) todo.Todo {
	return todo.Todo{
		ID:        r.Id,
		Title:     r.GetString("title"),
		Completed: r.GetBool("completed"),
		CreatedAt: r.GetDateTime("created").Time(),
		UpdatedAt: r.GetDateTime("updated").Time(),
	}
}

func (h *TodoHandler) renderTodoList(todos []todo.Todo) templ.Component {
	return components.TodoListRegion(todo.Signals{
		Todos: todos, Filter: "all", ItemCount: len(todos),
		AdminEnabled: h.cfg.AdminToken != "",
		LLMEnabled:   h.llmEnabled(),
	})
}
