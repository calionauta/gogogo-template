# Architecture — LLM Entrypoint

> This file is the **canonical entrypoint for LLM agents** navigating the template.
> Each row describes one component: where it lives, what it depends on, and how to remove or replace it.

---

## Principles

- **Unified build.** One `go build ./cmd/web` compiles everything. No build tags. Opt out at runtime with env vars (`NATS_ENABLED=false`, `DAGNATS_ENABLED=false`).
- **Features in `features/`**, infrastructure in `internal/`. Features depend on infra, never the reverse.
- **Wiring in `router/router.go` → `Init()`**. Every feature is registered by one function call.
- **Startup order in `cmd/web/main.go`** (see diagram below).

---

## Layer taxonomy

Every component belongs to one of three layers:

| Layer | Meaning | You would… |
|:------|:--------|:-----------|
| **Core** 🔴 | Binary does not work without it. | Customize, never remove. |
| **Infra** 🟡 | Binary works but loses capability if removed. | Swap for an equivalent. |
| **Plugin** 🟢 | A demo/add-on that plugs into the infra. Remove with no side effects. | Delete the package + remove the wiring call. |

---

## Features (features/)

These are **product-level demos** — what the end user sees. All are Plugin layer: you can remove, replace, or keep them.

| Feature | Package | Layer | Router wiring | Remove by |
|---------|---------|:-----:|---------------|-----------|
| **Auth** | `features/auth/` | 🟢/🔴 | `auth.RegisterAuth(se)` | Delete package + remove call |
| **Todo** | `features/todo/` | 🟢 | `todoH.RegisterRoutes(se)` | Delete package + remove block |
| **Whiteboard** | `features/whiteboard/` + `internal/collab/` | 🟢 | `registerWhiteboard(se, q)` | Delete both + `whiteboard.js` + remove call |
| **Onboarding** | `features/todo/handlers/onboarding.go` + `internal/dagnats/` | 🟢 | `registerOnboarding(...)` | Delete both + remove call |

> **⚠️ Auth is a mixed package.** The **login UI** (login page, navbar) is 🟢 Plugin — replace with OAuth, SSO, etc. The **auth middleware** (`LoadAuthFromCookie`) is 🔴 Core — the app's security model depends on it. They live in the same package for cohesion; if you replace the UI, keep the middleware functions.

---

## Infrastructure (internal/)

These are the **plumbing layers**. Each is independently replaceable.

| Component | Package | Layer | Startup | Swap / Remove by |
|-----------|---------|:-----:|---------|------------------|
| **PocketBase** (DB + Auth + API) | `db/` | 🔴 | `db.Init(cfg)` in `server.Run()` | Replace with your own DB + auth stack |
| **Config** | `config/` | 🔴 | `config.Load()` in `main.go` | Add/remove env vars |
| **Queue + SSE Hub** (goqite) | `internal/queue/` | 🔴 | `queue.New(cfg)` in `server.Run()` | Replace goqite with Redis; SSE Hub is in `ssehub.go` |
| **Event bus: NATS JetStream** | `internal/nats/` | 🟡 | `startNATS(cfg)` in `main.go` | Remove `startNATS` call; falls back to in-memory fan-out via SSE Hub |
| **DagNats** (workflows) | `internal/dagnats/` | 🟢 | `startDagNats(cfg, pb, ...)` in `main.go` | Remove call + delete package |
| **CRDT + Sync** (Loro) | `internal/collab/` | 🟢 | Via `registerWhiteboard` + `registerCollabSync` | Delete with whiteboard |
| **SSE helpers** (Datastar) | `internal/datastar/` | 🟡 | Imported by handlers | Replace with your own SSE rendering |
| **Secrets** (age) | `internal/secrets/` | 🟡 | `secrets.Load(appName)` in `config.Load()` | Remove call; env vars work without it |
| **LLM client** (GoAI) | `internal/llm/` | 🟢 | `llm.New(apiKey)` in `server.Run()` | Remove env var; UI auto-hides the Suggest button. *The package stays if you add your own AI feature — only the demo Suggest route is removable.* |

> **🔴 Core** = keep or replace the whole stack.  
> **🟡 Infra** = you could remove it and still serve pages, but lose cross-instance broadcast, async jobs, etc.  
> **🟢 Plugin** = pure add-on. Delete the package, remove the wiring call, nothing breaks.

### The three async layers — how they compose

```
                ┌──────────────────────────────────┐
                │  goqite queue (SQLite-backed)     │  ← Core: background jobs
                └─────────┬────────────┬───────────┘
                          │            │
                          ▼            ▼
              ┌──────────────────┐  ┌───────────────────┐
              │    SSE Hub       │  │  DagNats workflows│
              │ (in-process fan) │  │  (Plugin, :8090)  │
              └────────┬─────────┘  └────────┬──────────┘
                       │                     │
                       ▼                     ▼
              ┌──────────────────────────────────┐
              │         Browser SSE               │
              │        (Datastar)                 │
              └──────────────────────────────────┘

              ┌──────────────────────────────────┐
              │   NATS JetStream                 │  ← Infra: cross-instance event bus
              │                                   │
              │  ▲ WebSyncWorker publishes ops    │
              │  ▲ DagNats persists workflow      │
              │  ▼ SyncWorker receives ops         │
              └──────────────────────────────────┘
```

- **goqite** (Core 🔴): every feature's async work. Worker pool streams progress via SSE Hub.
- **SSE Hub** (Core 🔴, in `internal/queue/ssehub.go`): in-process fan-out to browser tabs via Go channels. Per-client channels, replay buffer, backpressure. **Does not cross the NATS boundary** — it is purely in-process.
- **NATS JetStream** (Infra 🟡): cross-instance event bus. Used by:
  - Whiteboard sync — shapes published directly via `WebSyncWorker.nc.Publish()` to subject `app.sync.<docID>` (bypasses SSE Hub entirely)
  - DagNats — workflow engine uses JetStream for durable state
  - Desktop Leaf Node — optional edge sync
  - *(Todos currently use the in-memory SSE Hub broadcaster, not NATS — the `JetStreamBroadcaster` exists in code but the startup order means it's never triggered. This is a pre-existing limitation: `server.Run(cfg, nil)` runs before `startNATS()`, so the router never receives a valid JetStream context. To fix it, either: (a) pass `startNATS()`'s JetStream context through `server.Run(cfg, js)`, or (b) follow the whiteboard's pattern of holding a direct `nc` connection reference.)*
- **DagNats** (Plugin 🟢): durable multi-step workflows as declarative JSON. Uses NATS JetStream for state.

---

## Startup order (cmd/web/main.go)

```
main.go
  ├─ config.Load()              ← 🔴 Core: reads env vars + age secrets
  ├─ server.Run(cfg, js)       ← 🔴 Core: PocketBase + queue + router.Init
  │   └─ router.Init(app, q, cfg, js, todoH)
  │       ├─ static files         🔴 Core
  │       ├─ auth.RegisterAuth    🟢/🔴 Plugin UI + Core middleware
  │       ├─ todo routes          🟢 Plugin
  │       ├─ registerOnboarding   🟢 Plugin
  │       ├─ registerWhiteboard   🟢 Plugin (creates DocStore)
  │       └─ registerCollabSync   🟢 Plugin (NATS listener)
  ├─ startDagNats(cfg, pb, ...) ← 🟢 Plugin: boots engine (NATS on :4222)
  ├─ startNATS(cfg)             ← 🟡 Infra: connects or starts embedded NATS
  └─ pb.Start()                 ← 🔴 Core: serves HTTP
```

> **Note on startup order:** `server.Run(cfg, nil)` is called BEFORE `startDagNats` and `startNATS`. This means `js` (the JetStream context) is always `nil` when `router.Init` runs. As a result, `newTodoBroadcaster(nil, hub)` always falls back to `InMemoryBroadcaster`. The whiteboard bypasses this limitation by publishing to NATS directly via `WebSyncWorker.nc.Publish()`. If you need NATS-backed todo broadcasting, pass the JetStream context through `server.Run(cfg, js)` instead of `nil`.

---

## Adding a new feature

1. Create `features/<name>/` with your HTTP handlers + Templ components.
2. Create a `RegisterRoutes(se, deps)` function that wires your routes.
3. Call it from `router/router.go` → `Init()`.
4. Add a "Remove by" comment.
5. Add a row to the Features table above.

New infra should go in `internal/<name>/`. Same pattern: create the package, wire it in `main.go` or `router.go`, add a "Swap / Remove by" comment.

---

## File tree

```
cmd/web/main.go            Entry point
config/config.go           Env-based config
db/pocketbase.go           PocketBase + seed
features/                  Demo features (all 🟢 Plugin)
  auth/                      Login/logout/cookie (UI 🟢, middleware 🔴)
  todo/                      Todo MVC (the reference implementation)
    handlers/                  HTTP routes + SSE stream + onboarding
    components/                Templ components (layout, todo_item, toast)
  whiteboard/                Collaborative canvas
internal/                  Infrastructure
  queue/                     goqite + SSE Hub + workers + retry + handler registry
  nats/                      Embedded NATS + JetStream broadcaster
  dagnats/                   DagNats client + workflow definitions
  collab/                    Loro CRDT + DocStore + sync workers + presence
  llm/                       GoAI LLM client + simulated client
  datastar/                  Datastar SSE rendering helpers
  secrets/                   age-decrypted secrets loader
router/router.go            Route wiring (central dependency graph)
web/resources/static/       Embedded JS/CSS assets
```
