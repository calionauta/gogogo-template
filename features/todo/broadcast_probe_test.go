package todo_test

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
	"time"
)

// TestTodoRecordsNotBroadcastViaHub is the regression guard for the (a)
// "best of both" realtime strategy: record mutations (create/toggle/delete)
// are NO LONGER pushed through the SSE hub. They now flow through
// PocketBase realtime, which is per-user scoped — so a peer client can NOT
// receive another user's record changes over the hub (the old cross-user
// leak). The hub keeps only ephemeral signals (retry feedback, suggest,
// workflow progress).
//
// Previously TestIntegration_BroadcastAcrossTwoClients asserted the OPPOSITE
// (peer B received the record via the hub). That behavior was the leak we
// removed, so this test now asserts the hub carries no record event.
func TestTodoRecordsNotBroadcastViaHub(t *testing.T) {
	base, _, _, _, cleanup := testFixture(t)
	defer cleanup()
	ctx := context.Background()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar: %v", err)
	}
	httpClient := &http.Client{Jar: jar, Timeout: 20 * time.Second}
	loginUser(ctx, t, httpClient, base, demoEmail, demoPassword)

	clientA := "rec-A-" + time.Now().Format(clientIDSuffixFormat)
	clientB := "rec-B-" + time.Now().Format(clientIDSuffixFormat)
	streamA := openSSEWithClient(ctx, t, httpClient, base, clientA)
	defer func() { _ = streamA.Body.Close() }()
	streamB := openSSEWithClient(ctx, t, httpClient, base, clientB)
	defer func() { _ = streamB.Body.Close() }()

	time.Sleep(200 * time.Millisecond)

	// Create a todo as client A (authed). With the old hub strategy this
	// would broadcast a "created" record event to client B (and exclude A).
	// With PocketBase realtime owning records, the hub carries NO record
	// event at all.
	createResp, err := doPostForm(ctx, httpClient,
		base+"/api/todos?clientID="+clientA, url.Values{titleField: {"hub-leak-check"}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_ = createResp.Body.Close()

	recordEvent := func(s string) bool {
		return strings.Contains(s, `"event":"created"`) ||
			strings.Contains(s, `"event":"toggled"`) ||
			strings.Contains(s, `"event":"deleted"`)
	}
	fullA := pumpSSEUntil(t, streamA, 6*time.Second, recordEvent)
	fullB := pumpSSEUntil(t, streamB, 6*time.Second, recordEvent)

	if recordEvent(fullA) {
		t.Fatalf("origin client wrongly received a record event via the hub "+
			"(records should be PB realtime):\n%s", tailString(fullA, 600))
	}
	if recordEvent(fullB) {
		t.Fatalf("peer client received a record event via the hub — cross-user leak NOT fixed:\n%s", tailString(fullB, 600))
	}
}

// TestTodoListFragment_ReturnsListRegion verifies the plain-HTML fragment
// endpoint the PocketBase-realtime client uses to morph #todo-list in place
// after a record change. It must render the list region (id="todo-list")
// containing the current todos, and require auth.
func TestTodoListFragment_ReturnsListRegion(t *testing.T) {
	base, _, _, _, cleanup := testFixture(t)
	defer cleanup()
	ctx := context.Background()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar: %v", err)
	}
	httpClient := &http.Client{Jar: jar, Timeout: 20 * time.Second}
	loginUser(ctx, t, httpClient, base, demoEmail, demoPassword)

	if _, perr := doPostForm(ctx, httpClient, base+"/api/todos",
		url.Values{titleField: {"frag-todo"}}); perr != nil {
		t.Fatalf("seed todo: %v", perr)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		base+"/api/todos/fragment", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("get fragment: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("fragment status=%d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read fragment: %v", err)
	}
	if !strings.Contains(string(body), `id="todo-list"`) {
		t.Fatalf("fragment did not render #todo-list region:\n%s", tailString(string(body), 300))
	}
	if !strings.Contains(string(body), "frag-todo") {
		t.Fatalf("fragment missing the created todo")
	}
}
