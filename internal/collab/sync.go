//go:build jetstream

package collab

import (
	"context"
	"strings"
	"sync"

	natsio "github.com/nats-io/nats.go"
)

// SyncWorker subscribes to collaborative doc updates on the JetStream
// subject app.sync.> and applies them to its local CRDT, then persists the
// resolved snapshot via the Persister. This is the server side of the
// edge-sync design: the desktop Leaf Node publishes Loro updates, they
// replicate to the central NATS, and this worker converges + saves them.
//
// Topic layout: "app.sync.<docID>" carries a Loro update for that doc.
type SyncWorker struct {
	nc        *natsio.Conn
	persister Persister

	mu   sync.Mutex
	docs map[string]*Doc
}

// NewSyncWorker builds a worker bound to a NATS connection and a
// Persister. The Persister is typically the PocketBase whiteboards
// collection (see pbpersist.go).
func NewSyncWorker(nc *natsio.Conn, p Persister) *SyncWorker {
	return &SyncWorker{
		nc:        nc,
		persister: p,
		docs:      make(map[string]*Doc),
	}
}

func (w *SyncWorker) doc(docID string) *Doc {
	w.mu.Lock()
	defer w.mu.Unlock()
	d, ok := w.docs[docID]
	if !ok {
		d = NewDoc(docID)
		w.docs[docID] = d
	}
	return d
}

// Run subscribes to app.sync.> and blocks processing updates until the
// context is cancelled (subscription drained) or the connection closes.
// Callers typically run it in a goroutine and cancel on shutdown.
func (w *SyncWorker) Run(ctx context.Context) error {
	sub, err := w.nc.Subscribe("app.sync.>", func(msg *natsio.Msg) {
		docID := docIDFromSubject(msg.Subject)
		if docID == "" {
			return
		}
		d := w.doc(docID)
		if err := d.ApplyUpdate(msg.Data); err != nil {
			// A malformed update is skipped — CRDT convergence must not
			// crash the worker on one bad message.
			return
		}
		// Persist the resolved snapshot as the source of truth.
		_ = w.persister.SaveSnapshot(docID, d.EncodeSnapshot())
	})
	if err != nil {
		return err
	}
	<-ctx.Done()
	return sub.Unsubscribe()
}

// docIDFromSubject extracts the doc id from "app.sync.<docID>".
func docIDFromSubject(subject string) string {
	parts := strings.Split(subject, ".")
	if len(parts) < 3 {
		return ""
	}
	return parts[2]
}
