package todo_test

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// sseTestTimeout caps how long any individual SSE-driven test waits for
// the worker pool to deliver. Generous enough to absorb CI noise without
// flaking local runs.
const sseTestTimeout = 5 * time.Second

// sseBufferSize is the read buffer for the SSE stream pump. Matches the
// goqite channel buffer so each Read pulls at most one full event.
const sseBufferSize = 4096

// clientIDSuffixFormat is a stable per-second suffix so the same test
// run yields stable clientIDs (useful when debugging SSE traffic dumps).
const clientIDSuffixFormat = "150405.000"

// TestIntegration_CreateEnqueuesNotification opens an SSE stream, creates
// a todo via HTTP, and asserts the "todo_created" notification arrives
// on the stream within a reasonable timeout. Exercises the full path:
//
//	HTTP POST → handler → goqite Enqueue → Hub Broadcast → SSE stream
func TestIntegration_CreateEnqueuesNotification(t *testing.T) {
	base, _, _, cleanup := testFixture(t)
	defer cleanup()

	ctx := newTestCtx(t)
	clientID := "test-client-" + time.Now().Format(clientIDSuffixFormat)

	stream := openSSE(t, base, clientID, sseTestTimeout)
	defer func() { _ = stream.Body.Close() }()

	// Give the SSE handler a beat to register the client with the hub
	// before the create handler fires its notification.
	time.Sleep(100 * time.Millisecond)

	mustPost(ctx, t, base, "/api/todos", url.Values{"title": {"eggs"}}, 200)

	full := pumpSSE(t, stream, sseTestTimeout, "todo_created")

	if !strings.Contains(full, "toast-container") {
		t.Fatalf("SSE notification missing #toast-container selector: %s", full)
	}
	if !strings.Contains(full, "eggs") {
		t.Fatalf("SSE notification missing todo title 'eggs': %s", full)
	}
	if !strings.Contains(full, "alert-success") {
		t.Fatalf("SSE notification missing alert-success class: %s", full)
	}
}

// TestIntegration_CreateEmitsToast verifies the asynchronous toast
// emitted by the worker after handleCreate enqueues a "todo_created"
// job. The HTTP response itself only patches the todo list; the toast
// arrives via the SSE stream once the worker picks up the job. This
// exercises the full HTTP → queue → worker → SSE pipeline that the
// SSE-aware retry path is designed for.
func TestIntegration_CreateEmitsToast(t *testing.T) {
	base, _, _, cleanup := testFixture(t)
	defer cleanup()

	clientID := "create-toast-client-" + time.Now().Format(clientIDSuffixFormat)

	ctx, cancel := context.WithTimeout(context.Background(), sseTestTimeout)
	defer cancel()

	stream := openSSEWithCtx(ctx, t, base, clientID)
	defer func() { _ = stream.Body.Close() }()

	// Give the SSE handler a moment to register the client with the Hub.
	time.Sleep(100 * time.Millisecond)

	// Trigger create with the matching clientID so the worker routes the
	// "todo_created" job to this specific stream. Hardcode localhost
	// for the create target so gosec's G107 (URL constructed from
	// untrusted string) doesn't trip on the dynamic clientID suffix.
	createURL := "http://127.0.0.1" + base[len("http://127.0.0.1"):] + "/api/todos?clientID=" + clientID
	resp, err := postForm(ctx, createURL, url.Values{"title": {"wash dishes"}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		t.Fatalf("create status=%d", resp.StatusCode)
	}

	full := pumpSSEUntil(t, stream, 2*time.Second, func(s string) bool {
		return strings.Contains(s, "wash dishes") && strings.Contains(s, "alert-success")
	})

	if !strings.Contains(full, "toast-container") {
		t.Fatalf("SSE toast missing #toast-container selector: %s", full)
	}
	if !strings.Contains(full, "toast-timer-bar") {
		t.Fatalf("SSE toast missing progress bar: %s", full)
	}
}

// parseSSEData extracts the `data:` payloads from a raw SSE transcript.
// Datastar emits `event: datastar\ndata: <payload>\n\n` blocks; we return
// the payload strings so callers can scan them for the lastRetry signal.
func parseSSEData(transcript string) []string {
	var out []string
	for _, block := range strings.Split(transcript, "\n\n") {
		for _, line := range strings.Split(block, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "data:") {
				out = append(out, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			}
		}
	}
	return out
}

// extractJSONString reads a JSON string literal starting at s. The
// caller passes s positioned just BEFORE the opening quote of the value
// (i.e. s begins with `":"<json>"`). It returns the unescaped
// contents and whether parsing succeeded. Handles escaped quotes so the
// embedded retry JSON (`"{\"attempt\":1}"`) is decoded correctly.
func extractJSONString(s string) (string, bool) {
	i := strings.Index(s, "\"")
	if i < 0 {
		return "", false
	}
	s = s[i+1:]
	var sb strings.Builder
	for j := 0; j < len(s); j++ {
		c := s[j]
		if c == '\\' && j+1 < len(s) {
			sb.WriteByte(s[j+1])
			j++
			continue
		}
		if c == '"' {
			return sb.String(), true
		}
		sb.WriteByte(c)
	}
	return "", false
}

// openSSE opens the SSE stream with a fresh context derived from the
// provided timeout. Used by tests that don't need to share the context.
func openSSE(t *testing.T, base, clientID string, timeout time.Duration) *http.Response {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	return openSSEWithCtx(ctx, t, base, clientID)
}

// openSSEWithCtx opens the SSE stream under the provided context. Used
// when the caller needs to share the context across multiple calls.
func openSSEWithCtx(ctx context.Context, t *testing.T, base, clientID string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, "GET", base+"/api/todos/stream?clientID="+clientID, nil)
	if err != nil {
		t.Fatalf("build SSE request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open SSE: %v", err)
	}
	if resp.StatusCode != 200 {
		_ = resp.Body.Close()
		t.Fatalf("SSE status=%d", resp.StatusCode)
	}
	return resp
}

// pumpSSE reads from the SSE stream until the predicate returns true
// or the timeout expires. Returns everything accumulated.
func pumpSSE(t *testing.T, stream *http.Response, timeout time.Duration, mustContain string) string {
	t.Helper()
	return pumpSSEUntil(t, stream, timeout, func(s string) bool {
		return strings.Contains(s, mustContain)
	})
}

// pumpSSEUntil reads the SSE stream until the predicate returns true
// or the timeout expires. The accumulated bytes are returned so
// callers can run multiple substring assertions on the full transcript.
func pumpSSEUntil(t *testing.T, stream *http.Response, timeout time.Duration, stop func(string) bool) string {
	t.Helper()
	buf := make([]byte, sseBufferSize)
	full := ""
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		n, err := stream.Body.Read(buf)
		if n > 0 {
			full += string(buf[:n])
			if stop(full) {
				return full
			}
		}
		if err != nil {
			break
		}
	}
	return full
}

// tailString returns the last n bytes of s, or all of s if shorter.
// Used by Logf calls so test output doesn't drown in stream dumps.
func tailString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
