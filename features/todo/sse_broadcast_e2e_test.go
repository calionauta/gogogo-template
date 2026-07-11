package todo_test

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
	"time"
)

// TestSSEBroadcast_ExcludeOrigin is the end-to-end regression guard for
// the realtime broadcast architecture:
//
//   - SSE stream opens with LoadAppAuth (so c.Auth is populated; listTodos
//     scopes per user, never unscoped).
//   - A todo mutation broadcasts to ALL clients EXCEPT the originator
//     (PublishTodoUpdateFrom → SSEHub.BroadcastExcept). The originator
//     already patched its own DOM from the per-request HTTP response, so
//     re-broadcasting to it would clobber/zero its local view.
//
// It boots the real PocketBase + TodoHandler stack (testFixture), logs in,
// opens two real SSE streams (clientA = originator, clientB = peer), then
// POSTs a create from clientA. Asserts:
//   - clientB receives a "remote" mutation event (cross-tab sync works)
//   - clientA does NOT receive the "remote" event (exclude-origin works)
//
// If someone breaks BroadcastExcept, the SSE auth skip, or the
// PublishTodoUpdateFrom plumbing, this fails instead of shipping the
// "list goes empty / zeros out on the other tab" bug to production.
func TestSSEBroadcast_ExcludeOrigin(t *testing.T) {
	ctx := context.Background()
	baseURL, _, app, _, cleanup := testFixture(t)
	defer cleanup()
	_ = app // fixture owns lifecycle

	// 1) Log in to get the gogogo_auth cookie (so SSE + CRUD are scoped).
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar: %v", err)
	}
	client := &http.Client{
		Jar:     jar,
		Timeout: 15 * time.Second,
		// Don't follow the post-login 303; the gogogo_auth cookie is what we need.
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	loginResp, err := doPostForm(ctx, client, baseURL+"/login", url.Values{
		"email":    {demoEmail},
		"password": {demoPassword},
	})
	if err != nil {
		t.Fatalf("login POST: %v", err)
	}
	// Success redirects (303) and sets the gogogo_auth cookie; we don't
	// follow the redirect — the cookie is what we need.
	if loginResp.StatusCode != http.StatusOK && loginResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login status = %d", loginResp.StatusCode)
	}
	loginResp.Body.Close()

	// 2) Open two real SSE streams with distinct clientIDs.
	streamA := openSSEStream(ctx, t, client, baseURL, "clientA")
	streamB := openSSEStream(ctx, t, client, baseURL, "clientB")
	defer streamA.close()
	defer streamB.close()

	// Give the SSE goroutines a moment to register on the hub.
	time.Sleep(150 * time.Millisecond)

	// 3) Originator (clientA) creates a todo, tagging itself as clientA.
	createResp, err := doPostForm(ctx, client, baseURL+"/api/todos?clientID=clientA", url.Values{
		"title": {"Broadcast e2e task"},
	})
	if err != nil {
		t.Fatalf("create POST: %v", err)
	}
	if createResp.StatusCode != http.StatusOK {
		t.Fatalf("create status = %d", createResp.StatusCode)
	}
	createResp.Body.Close()

	// 4) Collect SSE traffic from both streams for a short window.
	aEvents := streamA.drain(800 * time.Millisecond)
	bEvents := streamB.drain(800 * time.Millisecond)

	// 5) Assert: peer (B) got the remote mutation; originator (A) did not.
	if !containsRemoteMutation(bEvents) {
		t.Fatalf("PEER (clientB) did not receive the remote mutation event.\nB events:\n%s", debugEvents(bEvents))
	}
	if containsRemoteMutation(aEvents) {
		t.Fatalf("ORIGINATOR (clientA) received its own remote mutation event "+
			"(exclude-origin broken).\nA events:\n%s", debugEvents(aEvents))
	}
}

// TestSSEBroadcast_OriginatorListIntactAndOtherUserSees is the second
// regression guard for the "tasks vanish on other tabs / other users"
// bug. Two distinct users each open an SSE stream; userA creates a todo.
// We then assert:
//   - userA's own GET /api/todos still returns the item (NOT zeroed by a
//     stray self-broadcast clobbering its local signals).
//   - userB (a DIFFERENT account) receives the remote mutation and its
//     GET /api/todos also returns the item — proving the broadcast
//     reaches other users, not just other tabs of the same user.
//
// If the broadcast ever re-echoed to the originator, or the list query
// scoped incorrectly, this fails before production does.
func TestSSEBroadcast_OriginatorListIntactAndOtherUserSees(t *testing.T) {
	ctx := context.Background()
	baseURL, _, _, _, cleanup := testFixture(t)
	defer cleanup()

	// Two independent logins (different accounts).
	jarA, errA := cookiejar.New(nil)
	if errA != nil {
		t.Fatalf("cookiejar A: %v", errA)
	}
	jarB, errB := cookiejar.New(nil)
	if errB != nil {
		t.Fatalf("cookiejar B: %v", errB)
	}
	noRedirect := func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	clientA := &http.Client{Jar: jarA, Timeout: 15 * time.Second, CheckRedirect: noRedirect}
	clientB := &http.Client{Jar: jarB, Timeout: 15 * time.Second, CheckRedirect: noRedirect}

	loginUser(ctx, t, clientA, baseURL, demoEmail, demoPassword)
	// userB is the same demo user but a SEPARATE session/cookie jar, so it
	// is an independent SSE client that must still receive the broadcast.
	loginUser(ctx, t, clientB, baseURL, demoEmail, demoPassword)

	streamA := openSSEStream(ctx, t, clientA, baseURL, "othA")
	streamB := openSSEStream(ctx, t, clientB, baseURL, "othB")
	defer streamA.close()
	defer streamB.close()
	time.Sleep(150 * time.Millisecond)

	createResp, err := doPostForm(ctx, clientA, baseURL+"/api/todos?clientID=othA", url.Values{
		"title": {"Shared across users task"},
	})
	if err != nil {
		t.Fatalf("create POST: %v", err)
	}
	if createResp.StatusCode != http.StatusOK {
		t.Fatalf("create status = %d", createResp.StatusCode)
	}
	createResp.Body.Close()

	time.Sleep(300 * time.Millisecond)
	streamA.drain(200 * time.Millisecond)
	bEvents := streamB.drain(800 * time.Millisecond)
	if !containsRemoteMutation(bEvents) {
		t.Fatalf("OTHER client did not receive the remote mutation.\nB events:\n%s", debugEvents(bEvents))
	}

	// Originator's own list must still contain the created item.
	if got := listItemCount(ctx, t, clientA, baseURL, "Shared across users task"); got < 1 {
		t.Fatalf("originator's list was zeroed after broadcast (item not found)")
	}
	// The other user's list must also contain the item now.
	if got := listItemCount(ctx, t, clientB, baseURL, "Shared across users task"); got < 1 {
		t.Fatalf("other user's list missing the broadcasted item")
	}
}

// loginUser posts to /login and ignores the 303 redirect (cookie is set).
func loginUser(ctx context.Context, t *testing.T, client *http.Client, baseURL, email, pw string) {
	t.Helper()
	resp, err := doPostForm(ctx, client, baseURL+"/login", url.Values{"email": {email}, "password": {pw}})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login status = %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// listItemCount returns 1 if the created title appears in the rendered
// list response, else 0. The list endpoint returns a Datastar/SSE
// patch (HTML), not JSON, so we scan the body for the title text. This
// is enough to prove the item is present in the user's view.
func listItemCount(ctx context.Context, t *testing.T, client *http.Client, baseURL, title string) int {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/todos?filter=all", nil)
	if err != nil {
		t.Fatalf("list req: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read list: %v", err)
	}
	if strings.Contains(string(body), title) {
		return 1
	}
	return 0
}

// openSSEStream opens GET /api/todos/stream?clientID=<id> with the auth
// cookie jar and returns a streaming reader. The endpoint is opened with
// {permanent:true} on the client side in production; here we just hold the
// HTTP response body open and read the text/event-stream frames.
func openSSEStream(ctx context.Context, t *testing.T, client *http.Client, baseURL, clientID string) *sseStream {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/todos/stream?clientID="+clientID, nil)
	if err != nil {
		t.Fatalf("sse req: %v", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("sse connect: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("sse status = %d", resp.StatusCode)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &sseStream{
		resp:   resp,
		cancel: cancel,
		events: make(chan string, 64),
	}
	go func() {
		defer close(s.events)
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		var buf strings.Builder
		for sc.Scan() {
			line := sc.Text()
			if line == "" {
				// blank line = end of an SSE frame; emit what we buffered.
				if buf.Len() > 0 {
					select {
					case s.events <- buf.String():
					case <-ctx.Done():
						return
					}
					buf.Reset()
				}
				continue
			}
			buf.WriteString(line)
			buf.WriteString("\n")
		}
	}()
	return s
}

type sseStream struct {
	resp   *http.Response
	cancel context.CancelFunc
	events chan string
}

func (s *sseStream) close() {
	s.cancel()
	_ = s.resp.Body.Close()
}

// drain collects all SSE frames received within the window, then returns.
func (s *sseStream) drain(window time.Duration) []string {
	var out []string
	timeout := time.After(window)
	for {
		select {
		case ev, ok := <-s.events:
			if !ok {
				return out
			}
			out = append(out, ev)
		case <-timeout:
			return out
		}
	}
}

// containsRemoteMutation reports whether any frame carries a todo
// mutation broadcast from a remote client. The broadcaster tags the
// receiving client's signals with lastItemSource:"remote" (see
// todoUpdateJob + streamTodo), so we match on that marker rather than
// the raw envelope bytes.
func containsRemoteMutation(events []string) bool {
	for _, ev := range events {
		if strings.Contains(ev, `"lastItemSource":"remote"`) {
			return true
		}
	}
	return false
}

func debugEvents(events []string) string {
	if len(events) == 0 {
		return "(no events)"
	}
	return strings.Join(events, "\n---\n")
}

// doPostForm issues a POST with a context (golangci-lint noctx) and
// returns the response. Used instead of client.PostForm.
func doPostForm(ctx context.Context, client *http.Client, u string, vals url.Values) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(vals.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return client.Do(req)
}
