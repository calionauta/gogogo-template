package collab

import (
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/calionauta/gogogo-fullstack-template/internal/queue"
)

// Shape is one whiteboard primitive stored in the Loro CRDT. The server
// owns the Doc; the browser only sends ops and renders the plain JSON
// the server broadcasts, so the client needs NO JS CRDT library.
type Shape struct {
	ID     string    `json:"id"`
	Type   string    `json:"type"` // "rect" | "ellipse" | "line" | "pen"
	X      float64   `json:"x"`
	Y      float64   `json:"y"`
	W      float64   `json:"w"`
	H      float64   `json:"h"`
	Points []float64 `json:"points,omitempty"` // pen: flat [x0,y0,x1,y1,...]
	Color  string    `json:"color"`
	Author string    `json:"author,omitempty"`
}

// ShapeOp is a single mutation a browser sends. Op is "add" | "clear".
type ShapeOp struct {
	Op    string `json:"op"`
	Shape Shape  `json:"shape"`
}

// WebShapesEvent is the wire envelope broadcast to peers: the resolved
// shapes list after applying an op. Clients re-render from it.
type WebShapesEvent struct {
	Type   string  `json:"type"` // "shapes"
	Doc    string  `json:"doc"`
	From   string  `json:"from"`
	Shapes []Shape `json:"shapes"`
}

// WebSyncWorker is the web-only transport for collaborative docs. It
// mirrors the jetstream SyncWorker but uses the existing SSEHub fan-out
// (the same broadcast primitive the todo feature uses) instead of NATS.
// This keeps the whiteboard functional in the default build with ZERO
// extra infrastructure — no JetStream, no Leaf Node.
//
// Flow per doc:
//   - The browser POSTs its Loro update bytes to /api/whiteboard/<id>/update.
//   - The handler calls ApplyUpdate, which merges into the in-memory Doc
//     and persists the resolved snapshot via the Persister (PocketBase).
//   - The worker then broadcasts the same update to every OTHER connected
//     client on the doc's SSE stream, which merges it locally.
//
// Because Loro updates are commutative, applying them in any order
// converges — so a simple broadcast of raw update bytes is sufficient; no
// central ordering is required.
type WebSyncWorker struct {
	hub       *queue.SSEHub
	persister Persister

	mu   sync.Mutex
	docs map[string]*Doc
}

// NewWebSyncWorker builds a web transport worker bound to the SSE hub.
// The Persister is typically the PocketBase whiteboards collection.
func NewWebSyncWorker(hub *queue.SSEHub, p Persister) *WebSyncWorker {
	return &WebSyncWorker{
		hub:       hub,
		persister: p,
		docs:      make(map[string]*Doc),
	}
}

// doc returns the in-memory CRDT for docID, creating it on first use.
func (w *WebSyncWorker) doc(docID string) *Doc {
	w.mu.Lock()
	defer w.mu.Unlock()
	d, ok := w.docs[docID]
	if !ok {
		d = NewDoc(docID)
		w.docs[docID] = d
	}
	return d
}

// ApplyUpdate merges an incoming Loro update for docID, persists the
// resolved snapshot, and broadcasts the update to every other connected
// client on the doc's stream. fromClientID is the originator (excluded
// from the broadcast). Returns the merged snapshot length for callers
// that want to confirm progress.
func (w *WebSyncWorker) ApplyUpdate(docID, fromClientID string, update []byte) (int, error) {
	d := w.doc(docID)
	if err := d.ApplyUpdate(update); err != nil {
		return 0, err
	}
	snapshot := d.EncodeSnapshot()
	if w.persister != nil {
		if err := w.persister.SaveSnapshot(docID, snapshot); err != nil {
			slog.Warn("collab: persist snapshot failed", "doc", docID, "error", err)
		}
	}
	// Broadcast the raw update so peers can merge it directly. The
	// originator already applied it locally; exclude to avoid a
	// redundant re-merge (and a flicker).
	payload, err := json.Marshal(WebUpdateEvent{
		Type: "update",
		Doc:  docID,
		From: fromClientID,
		Data: update,
	})
	if err != nil {
		slog.Warn("collab: marshal update", "doc", docID, "error", err)
		return len(snapshot), err
	}
	w.hub.BroadcastExcept(payload, fromClientID)
	return len(snapshot), nil
}

// ApplyOp applies a shape op (add / clear) to the doc's LoroMap, persists
// the snapshot, and broadcasts the resolved shapes list to peers
// (excluding the originator). Returns the current shapes after the op.
func (w *WebSyncWorker) ApplyOp(docID, fromClientID string, op ShapeOp) ([]Shape, error) {
	d := w.doc(docID)
	shapes, err := d.ApplyShapeOp(op)
	if err != nil {
		return nil, err
	}
	snapshot := d.EncodeSnapshot()
	if w.persister != nil {
		if sErr := w.persister.SaveSnapshot(docID, snapshot); sErr != nil {
			slog.Warn("collab: persist snapshot failed", "doc", docID, "error", sErr)
		}
	}
	payload, err := json.Marshal(WebShapesEvent{
		Type:   "shapes",
		Doc:    docID,
		From:   fromClientID,
		Shapes: shapes,
	})
	if err != nil {
		slog.Warn("collab: marshal shapes", "doc", docID, "error", err)
		return shapes, err
	}
	w.hub.BroadcastExcept(payload, fromClientID)
	return shapes, nil
}

// Shapes returns the current resolved shape list for a doc.
func (w *WebSyncWorker) Shapes(docID string) []Shape {
	return w.doc(docID).Shapes()
}

// WebUpdateEvent is the wire envelope for a web transport update, sent
// (if a snapshot exists). Called when a client opens the doc so it starts
// from the latest saved state before receiving live updates.
func (w *WebSyncWorker) LoadSnapshot(docID string) ([]byte, bool) {
	if w.persister == nil {
		return nil, false
	}
	snap, ok := w.persister.LoadSnapshot(docID)
	if !ok {
		return nil, false
	}
	d := w.doc(docID)
	if err := d.ApplyUpdate(snap); err != nil {
		slog.Warn("collab: rehydrate failed", "doc", docID, "error", err)
		return nil, false
	}
	return snap, true
}

// WebUpdateEvent is the wire envelope for a web transport update, sent
// over the SSE hub to peers. Data is the raw Loro update bytes.
type WebUpdateEvent struct {
	Type string `json:"type"` // "update"
	Doc  string `json:"doc"`
	From string `json:"from"` // originator client id (for diagnostics)
	Data []byte `json:"data"` // Loro update bytes
}
