package collab

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	natsio "github.com/nats-io/nats.go"
)

// presenceSubject returns the pub/sub subject for a doc's presence.
// Exported so the central SSE bridge (router/collab_jetstream.go) can
// subscribe the same subject the edges publish to.
func PresenceSubject(docID string) string { return "app.presence." + docID }

// PresenceSSEHandler returns an http.HandlerFunc that streams a doc's
// presence events to browser clients over Server-Sent Events. It
// subscribes the app.presence.<docID> NATS subject (the same one desktop
// edges publish to, including Leaf Node replicas) and writes each event as
// an SSE `data:` line, flushing immediately. The connection closes when
// the client disconnects (request context done).
func PresenceSSEHandler(nc *natsio.Conn) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		docID := r.PathValue("docID")
		if docID == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher, ok := w.(http.Flusher)
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// Send headers immediately so the client's request returns and the
		// stream opens; an SSE comment also primes some proxies.
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, ": connected\n\n")
		flusher.Flush()
		ctx := r.Context()
		sub, err := nc.Subscribe(PresenceSubject(docID), func(m *natsio.Msg) {
			select {
			case <-ctx.Done():
				return
			default:
			}
			fmt.Fprintf(w, "data: %s\n\n", m.Data)
			flusher.Flush()
		})
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		defer sub.Unsubscribe()
		<-ctx.Done()
	}
}

// Presence provides ephemeral multi-user cursor/presence over NATS pub/sub.
// It is transport-agnostic: the same code runs on the desktop edge (over its
// Leaf Node connection, so cursors replicate to central and other edges) and
// on the central server (over the embedded NATS, so browser clients connected
// to central receive them). No PocketBase persistence — presence is volatile.
type Presence struct {
	nc        *natsio.Conn
	hb        time.Duration // heartbeat interval
	ttl       time.Duration // roster entry lifetime
	mu        sync.Mutex
	roster    map[string]PresenceMsg // user -> last seen cursor
	handlers  []func(PresenceMsg)
	heartbeat PresenceMsg
}

// NewPresence builds a presence session for one user on one doc. hb/ttl
// default to 3s/8s when zero. The heartbeat publishes a "join" on start and
// "leave" on stop so peers can maintain an accurate roster.
func NewPresence(nc *natsio.Conn, docID, user string, hb, ttl time.Duration) *Presence {
	if hb <= 0 {
		hb = 3 * time.Second
	}
	if ttl <= 0 {
		ttl = 8 * time.Second
	}
	return &Presence{
		nc:        nc,
		hb:        hb,
		ttl:       ttl,
		roster:    make(map[string]PresenceMsg),
		heartbeat: PresenceMsg{Type: "join", Doc: docID, User: user, TS: time.Now().UnixMilli()},
	}
}

// OnChange registers a callback fired for every presence event (including
// local expiry of stale peers). The UI uses it to render/move/remove cursors.
func (p *Presence) OnChange(fn func(PresenceMsg)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.handlers = append(p.handlers, fn)
}

// PublishCursor broadcasts the local user's cursor position (normalized
// 0..1). It also refreshes the local roster entry.
func (p *Presence) PublishCursor(x, y float64) error {
	msg := PresenceMsg{Type: "cursor", Doc: p.heartbeat.Doc, User: p.heartbeat.User, X: x, Y: y, TS: time.Now().UnixMilli()}
	p.mu.Lock()
	p.roster[msg.User] = msg
	p.mu.Unlock()
	return p.publish(msg)
}

// Subscribe starts receiving presence for the doc and begins heartbeating.
// It blocks until ctx is cancelled, then publishes a "leave" and returns.
func (p *Presence) Subscribe(ctx context.Context) error {
	sub, err := p.nc.Subscribe(PresenceSubject(p.heartbeat.Doc), func(m *natsio.Msg) {
		var msg PresenceMsg
		if err := json.Unmarshal(m.Data, &msg); err != nil {
			return
		}
		p.apply(msg)
	})
	if err != nil {
		return err
	}
	// Announce arrival.
	if err := p.publish(p.heartbeat); err != nil {
		slog.Warn("collab: presence join publish failed", "error", err)
	}

	expire := time.NewTicker(p.ttl / 2)
	beat := time.NewTicker(p.hb)
	defer expire.Stop()
	defer beat.Stop()
	defer func() {
		// Best-effort leave on shutdown.
		_ = p.publish(PresenceMsg{Type: "leave", Doc: p.heartbeat.Doc, User: p.heartbeat.User, TS: time.Now().UnixMilli()})
		_ = sub.Unsubscribe()
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-beat.C:
			// Re-affirm join so peers don't expire us during idle presence.
			_ = p.publish(PresenceMsg{Type: "join", Doc: p.heartbeat.Doc, User: p.heartbeat.User, TS: time.Now().UnixMilli()})
		case <-expire.C:
			p.expireStale()
		}
	}
}

// apply merges an incoming presence event into the roster and notifies.
func (p *Presence) apply(msg PresenceMsg) {
	p.mu.Lock()
	switch msg.Type {
	case "leave":
		delete(p.roster, msg.User)
	case "cursor", "join":
		p.roster[msg.User] = msg
	}
	handlers := append([]func(PresenceMsg){}, p.handlers...)
	p.mu.Unlock()
	for _, h := range handlers {
		h(msg)
	}
}

// expireStale drops roster entries older than ttl and notifies leaves.
func (p *Presence) expireStale() {
	now := time.Now().UnixMilli()
	p.mu.Lock()
	var expired []PresenceMsg
	for u, m := range p.roster {
		if u == p.heartbeat.User {
			continue // never expire self
		}
		if now-m.TS > p.ttl.Milliseconds() {
			delete(p.roster, u)
			expired = append(expired, PresenceMsg{Type: "leave", Doc: p.heartbeat.Doc, User: u, TS: now})
		}
	}
	handlers := append([]func(PresenceMsg){}, p.handlers...)
	p.mu.Unlock()
	for _, m := range expired {
		for _, h := range handlers {
			h(m)
		}
	}
}

func (p *Presence) publish(msg PresenceMsg) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return p.nc.Publish(PresenceSubject(msg.Doc), data)
}

// Roster returns a snapshot copy of currently-present peers (excluding self).
func (p *Presence) Roster() []PresenceMsg {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]PresenceMsg, 0, len(p.roster))
	for u, m := range p.roster {
		if u == p.heartbeat.User {
			continue
		}
		out = append(out, m)
	}
	return out
}
