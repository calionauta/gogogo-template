package handlers

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/pocketbase/pocketbase/core"
	sdk "github.com/starfederation/datastar-go/datastar"

	"github.com/calionauta/gogogo-fullstack-template/features/todo"
	dshelpers "github.com/calionauta/gogogo-fullstack-template/internal/datastar"
)

func (h *TodoHandler) handleList(c *core.RequestEvent) error {
	filter := c.Request.URL.Query().Get("filter")
	todos, err := h.listTodos(c, filter)
	if err != nil {
		slog.Error("todo: list failed", "filter", filter, "error", err)
		return c.String(statusInternal, "error listing todos")
	}

	sse := sdk.NewSSE(c.Response, c.Request)
	return dshelpers.MergeSignals(sse, todo.Signals{
		Todos: todos, Filter: filter, ItemCount: len(todos),
		AdminEnabled: h.cfg.AdminToken != "",
		LLMEnabled:   h.llmEnabled(),
	})
}

func (h *TodoHandler) handleCreate(c *core.RequestEvent) error {
	if err := c.Request.ParseForm(); err != nil {
		return c.String(statusBadRequest, "invalid form")
	}
	title := c.Request.FormValue("title")
	if title == "" {
		return c.String(statusBadRequest, "title required")
	}

	item := todo.Todo{
		Title:     title,
		Completed: false,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := h.saveTodo(&item, ownerOf(c)); err != nil {
		slog.Error("todo: save failed", "error", err)
		return c.String(statusInternal, "save failed")
	}

	h.broadcastTodo(c, "created", item)

	todos, err := h.listTodos(c, "all")
	if err != nil {
		slog.Error("todo: list after create failed", "error", err)
		return c.String(statusInternal, "error listing todos")
	}
	sse := sdk.NewSSE(c.Response, c.Request)
	// Render the updated list synchronously (the broadcaster already
	// re-renders other connected clients in real time). No queue here:
	// the create is fast and local, so the queue would only add latency.
	if err := dshelpers.RenderAndPatch(sse, h.renderTodoList(todos), sdk.WithSelector("#todo-list")); err != nil {
		return err
	}
	return emitToast(sse, "Added", "success")
}

func (h *TodoHandler) handleToggle(c *core.RequestEvent) error {
	rec, err := h.app.FindRecordById("todos", c.Request.PathValue("id"))
	if err != nil {
		return c.String(statusNotFound, "not found")
	}
	// Owner-scoped: a user can only mutate their own todos. Todos with
	// an empty owner (legacy seeds) are allowed so the single-tenant
	// demo keeps working.
	if c.Auth != nil && rec.GetString("owner") != "" && rec.GetString("owner") != c.Auth.Id {
		return c.String(statusNotFound, "not found")
	}
	rec.Set("completed", !rec.GetBool("completed"))
	if saveErr := h.app.Save(rec); saveErr != nil {
		slog.Error("todo: toggle save failed", "id", rec.Id, "error", saveErr)
		return c.String(statusInternal, "toggle failed")
	}

	toggled := todo.Todo{
		ID:        rec.Id,
		Title:     rec.GetString("title"),
		Completed: rec.GetBool("completed"),
	}
	h.broadcastTodo(c, "toggled", toggled)

	todos, err := h.listTodos(c, "all")
	if err != nil {
		slog.Error("todo: list after toggle failed", "error", err)
		return c.String(statusInternal, "error listing todos")
	}
	sse := sdk.NewSSE(c.Response, c.Request)
	return dshelpers.RenderAndPatch(sse, h.renderTodoList(todos), sdk.WithSelector("#todo-list"))
}

func (h *TodoHandler) handleDelete(c *core.RequestEvent) error {
	rec, err := h.app.FindRecordById("todos", c.Request.PathValue("id"))
	if err != nil {
		return c.String(statusNotFound, "not found")
	}
	// Owner-scoped: a user can only delete their own todos.
	if c.Auth != nil && rec.GetString("owner") != "" && rec.GetString("owner") != c.Auth.Id {
		return c.String(statusNotFound, "not found")
	}
	title := rec.GetString("title")
	if delErr := h.app.Delete(rec); delErr != nil {
		slog.Error("todo: delete failed", "id", rec.Id, "error", delErr)
		return c.String(statusInternal, "delete failed")
	}

	h.broadcastTodo(c, "deleted", todo.Todo{ID: rec.Id, Title: title})

	todos, err := h.listTodos(c, "all")
	if err != nil {
		slog.Error("todo: list after delete failed", "error", err)
		return c.String(statusInternal, "error listing todos")
	}
	sse := sdk.NewSSE(c.Response, c.Request)
	if err := dshelpers.RenderAndPatch(sse, h.renderTodoList(todos), sdk.WithSelector("#todo-list")); err != nil {
		return err
	}
	return emitToast(sse, fmt.Sprintf("Deleted “%s”", title), "info")
}

func (h *TodoHandler) handleClearCompleted(c *core.RequestEvent) error {
	records, err := h.app.FindRecordsByFilter("todos", clearCompletedFilter(c), "", 0, 0)
	if err != nil {
		slog.Error("todo: find completed failed", "error", err)
		return c.String(statusInternal, "find failed")
	}
	count := len(records)
	for _, r := range records {
		if delErr := h.app.Delete(r); delErr != nil {
			slog.Warn("todo: clear-completed delete failed", "id", r.Id, "error", delErr)
		}
	}

	todos, err := h.listTodos(c, "all")
	if err != nil {
		slog.Error("todo: list after clear failed", "error", err)
		return c.String(statusInternal, "error listing todos")
	}
	sse := sdk.NewSSE(c.Response, c.Request)
	if err := dshelpers.RenderAndPatch(sse, h.renderTodoList(todos), sdk.WithSelector("#todo-list")); err != nil {
		return err
	}
	if count == 0 {
		return emitToast(sse, "Nothing to clear", "info")
	}
	return emitToast(sse, fmt.Sprintf("Cleared %d completed", count), "success")
}

// ownerOf returns the authenticated user's id, or "" if the request is
// unauthenticated. Used to scope created todos to a tenant so the demo
// user sees only their own todos.
func ownerOf(c *core.RequestEvent) string {
	if c == nil || c.Auth == nil {
		return ""
	}
	return c.Auth.Id
}
