package router

import (
	"github.com/pocketbase/pocketbase/core"

	"github.com/calionauta/gogogo-fullstack-template/features/whiteboard"
	"github.com/calionauta/gogogo-fullstack-template/internal/collab"
	"github.com/calionauta/gogogo-fullstack-template/internal/nats"
	"github.com/calionauta/gogogo-fullstack-template/internal/queue"
)

// registerWhiteboard wires the collaborative whiteboard with NATS as the
// default cross-instance transport. Creates the shared DocStore that both
// the whiteboard's WebSyncWorker and the SyncWorker (collab sync) use, so
// browser ops and NATS-delivered updates converge the same in-memory docs.
//
// Returns the DocStore for registerCollabSync to use.
func registerWhiteboard(se *core.ServeEvent, q *queue.Queue) *collab.DocStore {
	docs := collab.NewDocStore()
	persister := collab.NewPocketBasePersister(se.App)
	nc := nats.Conn() // may be nil if NATS not started; WebSyncWorker handles nil
	h := whiteboard.New(se.App, q.Hub(), persister, docs, nc)
	h.RegisterRoutes(se)
	return docs
}
