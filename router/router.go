package router

import (
	"io/fs"
	"log"
	"net/http"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"

	"github.com/calionauta/gogogo-fullstack-template/config"
	"github.com/calionauta/gogogo-fullstack-template/features/auth"
	"github.com/calionauta/gogogo-fullstack-template/features/todo/handlers"
	"github.com/calionauta/gogogo-fullstack-template/internal/nats"
	"github.com/calionauta/gogogo-fullstack-template/internal/queue"
	"github.com/calionauta/gogogo-fullstack-template/web/resources"
)

// Init registers custom routes on PocketBase's serve event.
// Call before pb.Start(). Pass todoH as the
// same handler instance the caller used for RegisterHandlers so the
// routes and the SSE worker path share state.
func Init(
	app *pocketbase.PocketBase,
	q *queue.Queue,
	cfg *config.Config,
	js nats.JetStreamLike,
	todoH *handlers.TodoHandler,
) {
	app.OnServe().Bind(&hook.Handler[*core.ServeEvent]{
		Priority: -100,
		Func: func(se *core.ServeEvent) error {
			se.Router.BindFunc(auth.LoadAuthFromCookie)
			return se.Next()
		},
	})

	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		// Global auth middleware is bound by the priority -100 hook above.
		se.Router.GET("/health", func(c *core.RequestEvent) error {
			return c.String(200, "ok")
		})

		// Serve embedded static assets (CSS, JS, images).
		//
		// PocketBase registers a catch-all dashboard route
		// (`e.Router.GET("/{path...}", apis.Static(...))`) that requires
		// superuser auth and redirects unauthenticated requests to /login.
		// That catch-all SHADOWS any wildcard route we register (e.g.
		// /static/*) — the handler is never reached, so every /static/*
		// request 303-redirects to /login and the browser chokes on the
		// HTML response when it expected CSS/JS (strict MIME checking).
		//
		// Echo gives EXACT routes the highest priority, so we register
		// one exact route per embedded file under /static/. Exact routes
		// win over the catch-all, the assets are served with correct MIME
		// types, and no auth is required (the dashboard auth is per-route,
		// not a global guard — proven by /api/todos serving unauthed).
		staticFS := resources.StaticFS()
		if err := fs.WalkDir(staticFS, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			route := "/static/" + path
			se.Router.GET(route, func(c *core.RequestEvent) error {
				hs := http.StripPrefix("/static/", http.FileServer(http.FS(staticFS)))
				hs.ServeHTTP(c.Response, c.Request)
				return nil
			})
			return nil
		}); err != nil {
			log.Printf("static: error walking embedded assets: %v", err)
		}

		// Auth: login/logout/cookie middleware. Wires the demo login
		// page and ensures every request has e.Auth populated from the
		// pb_auth cookie before reaching feature handlers.
		auth.CookieSecure = !cfg.Dev && cfg.Host != "127.0.0.1"
		auth.RegisterAuth(se)
		// Remove auth: delete this line + delete features/auth/

		// Wire the realtime broadcaster so todo mutations are shared
		// with every connected client. Uses NATS JetStream when available,
		// falls back to in-memory SSE Hub fan-out. Built ONCE and shared
		// across all handlers to avoid "already bound" consumer errors.
		broadcaster := newTodoBroadcaster(js, q.Hub())
		if jsBroadcaster, ok := broadcaster.(interface{ Subscribe(*queue.SSEHub) }); ok {
			jsBroadcaster.Subscribe(q.Hub())
		}
		if todoH != nil {
			todoH.SetBroadcaster(broadcaster)
			todoH.RegisterRoutes(se)
		} else {
			// Defensive fallback: construct a fresh handler if the
			// caller forgot to pass one.
			fallback := handlers.New(app, q, cfg)
			fallback.SetBroadcaster(broadcaster)
			fallback.RegisterRoutes(se)
		}
		// Remove todos: delete this block + delete features/todo/ + delete db/pocketbase.go seed

		// Onboarding: DagNats durable workflow (WelcomeOnboarding).
		// Dependencies: DagNats running on :8090, TodoHandler, NATS broadcaster.
		registerOnboarding(app, q, se, broadcaster, todoH, cfg.DagNats.HTTPAddr)
		// Remove onboarding: delete this line + delete features/todo/handlers/onboarding.go + delete internal/dagnats/

		// Whiteboard: collaborative canvas (Loro CRDT + Rough.js + SSE hub + NATS sync).
		// Dependencies: SSE Hub, PocketBase whiteboards collection, NATS sync worker.
		// Creates the shared DocStore used by both WebSyncWorker and SyncWorker.
		docs := registerWhiteboard(se, q)
		// Remove whiteboard: delete this line + delete features/whiteboard/ + delete internal/collab/ + delete web/resources/static/whiteboard.js

		// Collab sync: subscribes app.sync.> on NATS, persists whiteboard docs.
		// Uses the same DocStore as the whiteboard handler (shared convergence).
		registerCollabSync(se, docs)
		// Remove collab sync: delete this line + delete internal/collab/sync.go

		return se.Next()
	})
}
