package llm

import (
	"context"
	"testing"

	"github.com/calionauta/gogogo-fullstack-template/internal/llm/fakeserver"
)

// TestChatSuggest_AISuggestPath verifies the real "AI Suggest" pathway
// (the Groq/GoAI client) returns 3 parsed suggestions when the provider
// answers with a JSON array. This is the backend half of the
// "AI Suggest" button on the Queue+Retry tab.
func TestChatSuggest_AISuggestPath(t *testing.T) {
	srv := fakeserver.NewServer(fakeserver.WithResponse(
		`["Write the integration tests","Fix the flaky retry","Review the open PR"]`))
	defer srv.Close()

	t.Setenv("GOAI_BASE_URL", srv.URL)
	// Disable the client's own retry so the test is deterministic.
	t.Setenv("GOAI_API_KEY", "test-key")

	c := New("test-key")
	if !c.Configured() {
		t.Fatal("client should be configured with a key")
	}

	out, err := c.ChatSuggest(context.Background(), "Plan my sprint")
	if err != nil {
		t.Fatalf("ChatSuggest failed: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("expected 3 suggestions, got %d: %v", len(out), out)
	}
	for i, s := range out {
		if s == "" {
			t.Fatalf("suggestion %d empty", i)
		}
	}
}

// TestChatSuggest_EmptyPartial ensures the real client rejects an empty
// title (the button posts $newTitle; if empty we must not call the API).
func TestChatSuggest_EmptyPartial(t *testing.T) {
	c := New("test-key")
	if _, err := c.ChatSuggest(context.Background(), ""); err == nil {
		t.Fatal("expected error on empty partial")
	}
}
