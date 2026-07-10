package components

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/calionauta/gogogo-fullstack-template/features/todo"
)

// TestRender_TodoItemCheckboxReflectsCompleted is a regression guard for the
// inverted-checkbox bug: the demo loaded with one task, but toggling it
// surfaced several others because the checkbox state was rendered
// inverted / the create form double-submitted. The checkbox must be
// server-rendered from item.Completed so the UI matches the data on
// first paint (no client-side flip, no phantom rows).
func TestRender_TodoItemCheckboxReflectsCompleted(t *testing.T) {
	done := todo.Todo{ID: "abc123", Title: "finished", Completed: true}
	open := todo.Todo{ID: "def456", Title: "still open", Completed: false}

	var bDone, bOpen bytes.Buffer
	if err := TodoItem(done).Render(context.Background(), &bDone); err != nil {
		t.Fatalf("render done item: %v", err)
	}
	if err := TodoItem(open).Render(context.Background(), &bOpen); err != nil {
		t.Fatalf("render open item: %v", err)
	}
	if !strings.Contains(bDone.String(), "checked") {
		t.Errorf("completed todo must render a checked checkbox; got:\n%s", bDone.String())
	}
	if strings.Contains(bOpen.String(), "checked") {
		t.Errorf("open todo must NOT render a checked checkbox; got:\n%s", bOpen.String())
	}
	// The toggle posts to the item's own id — a regression that pointed
	// at a shared/hardcoded id would double-toggle or hit the wrong row.
	if !strings.Contains(bDone.String(), "/api/todos/abc123/toggle") {
		t.Errorf("done item toggle must target its own id (abc123)")
	}
	if !strings.Contains(bOpen.String(), "/api/todos/def456/toggle") {
		t.Errorf("open item toggle must target its own id (def456)")
	}
}

// TestRender_QueueRetryButtonEnabled is a regression guard for the
// "suggest simulated button came disabled" bug. The single "Queue + retry"
// affordance must render ENABLED (no static disabled attribute) so the
// goqite + retry-go + fake-LLM demo is usable out of the box. Only
// signal-driven disabling (e.g. while onboarding runs) is acceptable.
func TestRender_QueueRetryButtonEnabled(t *testing.T) {
	signals := todo.Signals{
		Todos:            nil,
		Filter:           "all",
		ItemCount:        0,
		ConnectedClients: 1,
		Suggestions:      []string{},
		SimulatedLLM:     true,
	}
	var buf bytes.Buffer
	if err := TodoList(signals).Render(context.Background(), &buf); err != nil {
		t.Fatalf("render list: %v", err)
	}
	html := buf.String()

	if !strings.Contains(html, "Queue + retry") {
		t.Fatalf("Queue + retry button missing from rendered UI")
	}
	// The button itself must not carry a hard-coded disabled attribute;
	// only the data-attr:disabled with a *signal* expression is allowed
	// (the create form uses $loading || !$newTitle.trim(), not a literal).
	if strings.Contains(html, `disabled="disabled"`) || strings.Contains(html, `disabled disabled`) {
		t.Errorf("Queue + retry button has a static disabled attribute; it must be enabled by default")
	}
}
