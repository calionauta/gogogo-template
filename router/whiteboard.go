package router

import (
	"github.com/pocketbase/pocketbase/core"

	"github.com/calionauta/gogogo-fullstack-template/features/whiteboard"
	"github.com/calionauta/gogogo-fullstack-template/internal/collab"
	"github.com/calionauta/gogogo-fullstack-template/internal/queue"
)

// registerWhiteboard wires the web-only collaborative whiteboard. It runs
// in EVERY build (no JetStream required): the transport is the shared SSE
// hub, so the whiteboard works out of the box. The jetstream-tagged
// registerCollabSync is the desktop-edge sync path; both can coexist.
//
// The persister writes the resolved Loro snapshot to the PocketBase
// "whiteboards" collection, which db/seed.go creates on boot.
func registerWhiteboard(se *core.ServeEvent, q *queue.Queue) {
	persister := collab.NewPocketBasePersister(se.App)
	h := whiteboard.New(se.App, q.Hub(), persister)
	h.RegisterRoutes(se)
}
