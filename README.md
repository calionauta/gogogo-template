# gogogo-fullstack-template

<p align="center">
  <img src="web/resources/static/logo.png" alt="gogogo-fullstack-template" width="512">
</p>

A starting point for web projects in Go. Single binary, zero external services, LLM-friendly.

We built this template to resolve those choices up front, without locking you into a closed ecosystem.

> **One binary, one process, one image.** ~30 MB, no shell, no libc, no CDN.
> Runs on `scratch` (or `gcr.io/distroless/static-debian12:nonroot` if you need a debug base). All CSS is compiled at build time via Tailwind v4 + DaisyUI v5 and embedded via `//go:embed` — no JS runtime.

> **Made with intent to be useful, not to be right.** We optimize for shipping something that runs today over being philosophically correct. Decisions here are pragmatic, not dogmatic.

## Contents

- [Who this template is for](#who-this-template-is-for)
- [What's in the package](#whats-in-the-package)
- [Stack in layers, not silos](#stack-in-layers-not-silos)
- [The example: Todo App with realtime](#the-example-todo-app-with-realtime)
- [PocketBase admin UI (built in)](#pocketbase-admin-ui-built-in)
- [Try it live](#try-it-live)
- [Configuring the LLM (GoAI)](#configuring-the-llm-goai)
- [Getting started](#getting-started)
- [Desktop & Mobile](#desktop--mobile-wails-v3--loro-crdt--nats-leaf-node)
- [Deploy to your own box](#deploy-to-your-own-box)
- [Structure](#structure)

---

Every web project we start begins with the same conversation: pick a database, auth, router, reactive UI framework, task queue… and the project stalls at the decisions, installations, and configurations — not the code.

## Who this template is for

- **You who get tired of configuring the same stack over and over**
- **You who want a single binary for deploy, with no Redis, Postgres, or SaaS**
- **You who want an LLM client wired in without pulling in a whole orchestration framework** — `internal/llm` wraps GoAI (any OpenAI-compatible provider) behind an injectable interface, callable from handlers. It calls a *remote* provider API; it is **not** a local-model runtime.
- **You who prefer server-rendered HTML over 2MB SPAs**

It's not a framework. There's no lock-in. Each piece can be replaced individually.

## What's in the package

Everything you need to build a modern web app, in a single binary:

| Layer | Choice | Why |
|-------|--------|-----|
| **Language** | Go 1.26 | Fast compilation, easy deploy, lean runtime |
| **Database + Auth + API** | [PocketBase](https://pocketbase.io) (embedded, on `ncruces/go-sqlite3`) | Zero-config auth, REST, [admin UI at `/_/`](https://<your-domain>/_/), file storage — all in SQLite |
| **Templating** | [Templ](https://templ.guide) | Type-safe Go components, generated at build time |
| **Reactive UI** | [Datastar](https://data-star.dev) (SSE) | Server-rendered over SSE, single ~12 KiB client. CSS built once via Tailwind v4 CLI; no JS runtime. |
| **CSS** | [DaisyUI v5](https://daisyui.com) + TailwindCSS | Ready components, customizable, ~34kB minified |
| **Task queue** | [goqite](https://github.com/maragudk/goqite) + SSE Hub | Background jobs streamed to the browser, no Redis |
| **Retries** | [avast/retry-go v4](https://github.com/avast/retry-go) | Exponential backoff with jitter, no boilerplate |
| **Workflows** | [DagNats](https://github.com/danmestas/dagnats) | Multi-step durable workflows as declarative JSON over NATS JetStream |
| **LLM SDK** | [GoAI](https://github.com/zendev-sh/goai) | Any provider: OpenAI, Anthropic, Groq, Ollama… |
| **Real-time** | [NATS JetStream](https://nats.io) | Multi-user real-time, cross-instance broadcast |
| **Secrets** | [age](https://age-encryption.org) + `~/.secrets/` | Local encryption, no vault, no cloud |
| **IDs** | [google/uuid](https://github.com/google/uuid) | Stable request/job IDs |
| **Live reload** | [Air](https://github.com/air-verse/air) | `make dev` regenerates templ and restarts the binary |
| **CRDT (collaborative docs)** | [loro-go](https://github.com/aholstenson/loro-go) | Conflict-free merging of whiteboard/notes state; converges offline edits with no LWW data loss |
| **Hand-drawn canvas** | [Rough.js](https://roughjs.com) (CDN) | Minimalist sketchy whiteboard rendering, loaded from jsDelivr — no build dependency |
| **Linting** | [golangci-lint](https://golangci-lint.run) + [datastar-lint](https://github.com/calionauta/datastar-lint) | `errcheck`, `staticcheck`, `gosec`, `revive`, `gocritic` (see `.golangci.yml`); `datastar-lint` catches Datastar attribute/signal/expression mistakes (run via `make datastar-lint`) |
| **CI/CD** | GitHub Actions | `ci.yml` (lint + test + build, unified build) + `deploy.yml` (multi-arch Docker to ghcr.io, runs on `master`) |

> **Why `ncruces/go-sqlite3`?** It's the pure-Go (no cgo) SQLite build that bundles the extensions this template leans on — FTS5, `spellfix1`, `unicode` collations — which the stock driver leaves out. That's why the `//go:build`/driver init in `db/pocketbase.go` pins it instead of `modernc.org/sqlite` directly.

## Stack in layers, not silos

Most templates force you to pick one async strategy — usually a queue, sometimes a workflow runtime, rarely both. Real apps need **a queue for background jobs**, **a workflow runtime for durable multi-step processes**, and **a real-time layer for cross-client state** — each solving a different problem. We ship all three in one unified build.

We solve this with **three complementary async layers**:

```
goqite    → background jobs + SSE to the browser (always on)
dagnats   → durable multi-step workflows as JSON (always on, runtime opt-out with DAGNATS_ENABLED=false)
JetStream → multi-user real-time (always on, runtime opt-out with NATS_ENABLED=false)
```

They coexist in the same binary. They don't compete.

**One `make build` compiles everything.** No build tags, no stub files, no matrix. Opt out at runtime via env vars: `NATS_ENABLED=false`, `DAGNATS_ENABLED=false`. DagNats and NATS share a single embedded JetStream on `:4222`.

## Feature overview

Every capability is always compiled. What you get:

| Capability | Runtime opt-out | What it does |
|-----------|----------------|--------------|
| **Todo app + PocketBase realtime** | — | DB actions (create/toggle/delete) stream through PocketBase realtime, per-user scoped via `owner` rule. SSE Hub for ephemeral signals (toasts, clients count, AI suggest) |
| **Queue + retry** | — | `goqite` background jobs + `retry-go` (the "Queue + Retry" demo) |
| **AI Suggest** | `GOAI_API_KEY` unset | GoAI/Groq call from the todo UI; button hidden when no key |
| **Collaborative whiteboard** | — | Loro CRDT + Rough.js canvas, SSE + NATS broadcast, offline-first outbox replay, PocketBase-persisted snapshots |
| **Multi-instance real-time** | `NATS_ENABLED=false` | NATS JetStream fan-out for todo + whiteboard sync across >1 instance behind a LB |
| **Durable workflows** | `DAGNATS_ENABLED=false` | DagNats JSON workflows over JetStream on `:8090` (e.g. `WelcomeOnboarding`) |
| **Desktop-edge sync** | `NATS_LEAFNODE_URL` unset | Leaf-Node JetStream replication of Loro updates for desktop/edge clients |

> **Adding a new feature?** Create `features/<name>/` with your handlers + templates. Wire it in `router/router.go` → `Init()` with a single function call. See `ARCHITECTURE.md` for the full pattern.

## The example: Todo App with realtime

We ship a working Todo App:

- Full CRUD via PocketBase
- Reactive UI with Datastar + DaisyUI
- **Database actions stream through PocketBase realtime.** Todo `create`/`toggle`/`delete` fire PocketBase record events; each subscribed client re-fetches the fragment and morphs `#todo-list`. Delivery is per-user scoped by the collection's `owner` rule (`@request.auth.id != '' && owner = @request.auth.id`), so a client only receives events for its own records. The SSE Hub is reserved for ephemeral signals (success/retry toasts, live clients count, AI suggest) and the originating client's own synchronous patch.
- Stacked toast notifications (auto-dismiss, manual close, progress bar)
- Async jobs: `handleCreate` enqueues a `todo_created` job; a worker picks it up and streams a success toast to the right browser tab via the SSE Hub (`clientID` routing)
- Retries with exponential backoff and jitter (`internal/queue/retry.go`, retry-go v4) — SSE-aware: a retry emits a `lastRetry` signal so the UI can show "retrying…"
- `WelcomeOnboarding` DagNats workflow (always compiled) that creates 3 example todos via durable steps — kill the server mid-run, restart, watch it resume at the last incomplete step. The workflow is declarative JSON (`internal/dagnats/workflow.go`), so renaming Go handlers never orphans an in-flight run.
- **Admin unlock** via `age` + `~/.secrets/`. The Todo example wires a master-password path: when `ADMIN_UNLOCK_TOKEN` is set (in the age-encrypted secrets file), the UI shows a "Clear all" form; the handler compares constant-time and clears all todos on match. Demonstrates the age flow end-to-end.
- **AI suggest** via GoAI. When `GOAI_API_KEY` is set, the input gets a "Suggest" button that enqueues an async suggest job (see queue below) and streams the 3 completions back via SSE. It talks to whatever OpenAI-compatible provider `GOAI_BASE_URL`/`GOAI_MODEL` point at — see [Configuring the LLM](#configuring-the-llm-goai). Retries with exponential backoff use the same `internal/queue/retry.go` as the SSE toast path. For a **keyless** demo of the exact same queue + retry path, `SIMULATE_LLM` is on by default (opt out with `SIMULATE_LLM=false`): a "Suggest (simulated)" button enqueues a job that hits an in-process fake LLM scripting 500 → 200 + delay, so you can watch the retry feedback toasts (enqueued → attempt failed → slow → result).
- Tests run with `-race`

> **This is the contract you should imitate when adding a new feature:**
> 1. **Pure HTTP + Datastar** for the user-facing surface.
> 2. **goqite job** for any work that takes more than ~50ms (LLM, email, exports).
> 3. **SSE toast** for async feedback to the originating client via `clientID` routing.
> 4. **age-encrypted secret** if the feature needs a credential.
> Every existing feature (toast on create, AI suggest, admin unlock, DagNats onboarding) follows this exact shape.

Enough to understand the pattern and start your own feature module.

## PocketBase admin UI (built in)

PocketBase ships a full admin UI for free — it runs at **`/_/`** on the same domain. Once deployed, your admin UI lives at `https://<your-domain>/_/`.

What you get out of the box:

- **Visual data browser** for every collection (todos, users, etc.) with sort/filter/CSV export
- **REST + JS SDK playground** for the API endpoints PocketBase generated from your schema
- **Superuser management** (create the first one via the install link printed in the server logs)
- **File storage** (S3-compatible uploads, images, attachments)
- **Backups** (SQLite snapshot, download + restore)
- **Logs** (requests, errors, slow queries)

This is **not a custom admin panel** — it's the upstream PocketBase UI, embedded in the same binary, on the same port. No extra service to deploy, no extra auth to wire. For production, point a Cloudflare Tunnel / Caddy ingress at the same `/_/` path and lock it down (IP allowlist, oauth2-proxy in front, or just PocketBase's own superuser auth).

### App session cookie vs `pb_auth` (why two)

The app **never** reuses PocketBase's own `pb_auth` cookie. PocketBase keeps the superuser (`_superusers`) and regular users as **separate auth namespaces with different endpoints**, and a single client holds only **one** auth state (one cookie). Sharing `pb_auth` for the app session clobbers the admin session in the same browser (and vice-versa) — a well-known PocketBase gotcha ([#5050](https://github.com/pocketbase/pocketbase/issues/5050), [#1780](https://github.com/pocketbase/pocketbase/issues/1780)).

So login issues **two** cookies:

- `gogogo_auth` — the app's own session cookie, read by `LoadAuthFromCookie`.
- `pb_auth` — the same token under PocketBase's native name, so PB-native surfaces (notably the `/api/realtime` SSE channel for record-change subscriptions) authenticate as the same user. Without it, realtime record events are silently dropped by PB's per-subscriber access check.

The split is **intentional, not tech debt** — keep the two cookies separate. **Best practice:** run the admin UI on a separate origin/port (e.g. `:8090/_/`) so even `pb_auth` never collides between admin and app.

## Try it live

A running deployment of this exact template is live. You can touch every feature from the README without cloning:

| What | URL | What you can do |
|------|-----|-----------------|
| **Todo demo app** | `https://<your-demo-domain>/` | Log in with the seeded demo account (`demo@demo.app` / `demo`), then watch the **event-driven DagNats onboarding** kick off automatically: step 1 greet → step 2 awaits your first todo (blocks on a `WaitForSignal`) → create a todo → the signal resumes the flow → steps 3–5 create 3 example todos + complete. Per-user (only your browser session is scoped to you, not global), event-driven (the create-todo event signals the waiting step), durable (a crash mid-wait resumes on restart because the signal KV retains the value). With `GOAI_API_KEY` set, the AI "Suggest" button appears; with `SIMULATE_LLM` on (default), a keyless "Suggest (simulated)" button exercises the same queue + retry path against an in-process fake LLM (error → retry → slow). Open two browser tabs to see the **self vs. remote** animations: items you create slide in from the top with a primary tint; items other tabs create slide in from the left with an info pulse + "from someone else" indicator. Delete and reorder use the browser View Transitions API for a free cross-fade. |
| **Live PocketBase admin dashboard** | `https://<your-demo-domain>/_/` | Open the embedded PocketBase UI to browse the `todos` + `users` collections, run the REST/JS SDK playground, and inspect logs. The demo's `users` collection is **locked** — visitors can log in as the demo user but cannot create or delete accounts through the API or this dashboard (only the superuser can). |
| **Durable workflow engine (DagNats)** | `https://<your-demo-domain>:8090` | The DagNats HTTP API where the `WelcomeOnboarding` workflow runs (declarative JSON over NATS JetStream). Inspect runs/steps or trigger them via the API; the Todo demo drives it automatically on first login. |

> The demo runs the unified build (everything compiled in). DagNats + NATS share a **single embedded JetStream** on `:4222` — DagNats boots it and the whiteboard SyncWorker attaches to it, so there is only one NATS process in the binary. To stand up your own, see [Deploy](#deploy).

## Configuring the LLM (GoAI)

The AI Suggest + Queue/Retry demo is wired through [GoAI](https://github.com/zendev-sh/goai) and reads its configuration from the environment (or your age-encrypted secrets file). Two paths:

**1. A real OpenAI-compatible provider (recommended for production).** Set `GOAI_API_KEY`, point `GOAI_BASE_URL` at the provider's `/v1` endpoint, and pick a `GOAI_MODEL`. Any OpenAI-compatible endpoint works — we do not hardcode a provider, you choose. With a key present, the Todo UI shows the **Suggest** button.

```bash
GOAI_API_KEY=sk-...
GOAI_BASE_URL=https://api.groq.com/openai/v1
GOAI_MODEL=llama-3.3-70b-versatile
```

**2. Keyless simulated LLM (on by default — best for trying the queue + retry path in dev).** `SIMULATE_LLM` is enabled automatically (no API key needed); set `SIMULATE_LLM=false` to disable it. It spins up an in-process fake GoAI client that scripts a realistic failure (500 → retry → slow → 200) so you can watch the retry feedback toasts end-to-end. The UI shows a **Suggest (simulated)** button that reuses the exact same `goqite` + `retry-go` path as the real provider.

If neither `GOAI_API_KEY` is set nor `SIMULATE_LLM` is enabled (i.e. `SIMULATE_LLM=false` and no key), the AI suggest route is **not registered** and the UI button is hidden. The Todo example keeps working — AI is opt-in, not required.

## Getting started

Use this template (green **Use this template** button above) or clone it:

```bash
git clone https://github.com/calionauta/gogogo-fullstack-template.git my-project
cd my-project
make dev
```

Open `http://localhost:8080` and see the Todo App running.

> The default port is `8080` (override with `PORT`). The default branch is `master`.

### Other commands

```bash
make build      # Build binary (unified: everything included)
make test       # Run tests with race detector (-p 1 for DagNats engine stability)
make check      # Lint + size limits + dead code + CSS check + tests
make css        # Build app.min.css (Tailwind v4 + DaisyUI v5)
make dev        # Live reload with Air
make templ      # Regenerate templ components
make setup      # Install pre-commit hooks
make docker-image  # Build and push multi-arch image to ghcr.io
```

### Build pipeline

The single compile step **outside** `go build` is the CSS build. It runs
once in the Docker builder stage and the result is embedded into the Go
binary via `//go:embed` — there is no runtime CSS build step, no JS
runtime, and no CDN.

```
src/css/input.css  →  tailwindcss v4 CLI  →  web/resources/static/app.min.css
                                                    │
                                                    └─ //go:embed in the Go binary
```

The pre-commit hook regenerates `app.min.css` automatically whenever
`.templ` or `.go` files change, and `make check` includes a `css-check`
step that fails the gate if the working CSS file is out of date.

## Local CI (gh-signoff)

Pushing to `master` triggers GitHub Actions (`ci.yml` runs the full gate,
then `deploy.yml` ships to production). You can run that **exact
gate on your own machine** before pushing, so you don't wait on remote
runners (and don't push a broken commit).

We use [gh-signoff](https://github.com/basecamp/gh-signoff) — a GitHub CLI
extension that stamps a green commit status after your local tests pass.

```bash
# one-time: install the extension
gh extension install basecamp/gh-signoff

# before pushing: run the full CI gate locally, then stamp the commit
# green (force so it stamps even before the commit is pushed)
make signoff
```

`make signoff` runs `make ci-local` (templ generate → golangci-lint →
datastar-lint → CSS check → `go test -race -p 1` → `go build`) and then
stamps the current commit green with `gh signoff`. `make ci-local` uses
`golangci-lint` as the authoritative formatter/lint gate (the same linter
CI runs) rather than the standalone `gofumpt` binary, which can be a newer
release than the one golangci-lint bundles and would otherwise produce
false-positive listings.

> **Advisory status, by design.** This repo deploys on **push to `master`**
> (not PR merge), so the signoff status is a *signal*, not a hard gate.
> We do not run `gh signoff install` (which would gate PR merges) — it would be meaningless for a push-to-deploy flow, so we leave it off by design.

## Desktop & Mobile (Wails v3 + Loro CRDT + NATS Leaf Node)

The same Go backend (PocketBase + queue + router + handlers) also runs as a
**native desktop app** via Wails v3. The desktop build reuses 100% of the
business logic — it boots `internal/server.Run`, serves PocketBase in a
goroutine, and points the webview at it through a reverse proxy.

Build commands (Wails v3 CLI):

```bash
# Current platform
wails3 build
# Cross-platform
wails3 build GOOS=windows
wails3 build GOOS=linux
wails3 build GOOS=darwin GOARCH=arm64
# macOS .app bundle
wails3 package GOOS=darwin
# Android APK (needs Android SDK/NDK + JDK 21 — see Mobile below)
wails3 android:package
```

All builds compile with everything included (unified build).
If you prefer a plain binary without the wails CLI, `make desktop` runs
`go build ./cmd/desktop`.

**Edge sync.** If `NATS_LEAFNODE_URL` is set, the desktop boots as a **NATS Leaf Node** that
syncs its JetStream streams with your central server — offline edits replay
on reconnect. Without it, it runs a standalone embedded NATS for local
realtime. On top of that transport, **Loro CRDT** collaboration
(`internal/collab`) publishes whiteboard updates on `app.sync.<docID>` and
ephemeral multi-user **cursors** on `app.presence.<docID>`; the central
server persists resolved Loro snapshots to PocketBase (`whiteboards`
collection) and streams presence to browser clients via SSE
(`GET /api/collab/presence/{docID}`).

**Desktop builds are local-only — not in CI.** Generate them with the
commands above: `wails3 build` for a binary, `wails3 package GOOS=darwin`
for a macOS `.app` (wrap in a `.dmg` with `hdiutil` if you want a
redistributable installer). Keeping the desktop out of CI keeps the pipeline
lean; the full e2e gate (incl. `TestCollab_LeafNodeE2E` and
`TestPresence_SSEBridgeE2E`) still runs in `ci.yml` under the unified build.

> **Mobile (Android) is opt-in, not in CI.** Wails v3 targets Android from
the same `main.go` (Go → `libwails.so`, WebView frontend) — no separate
mobile project. Generate an APK locally with `wails3 android:package`
(or `android:package:fat`). This requires the **Android SDK (API 35) +
NDK (26.3.x) + JDK 21**; `wails3 doctor` reports what's missing. Because
that toolchain is heavy, APK builds are left to the developer and are not
part of the CI matrix. iOS is analogous but requires Xcode.

## Deploy to your own box

The default workflow is to **clone + `make dev`** for local work. For a permanent
demo, the project ships a production deploy workflow that publishes to a
server of your choosing (recommended: a small Linux box + Tailscale + a
Cloudflare-tunneled domain). No registry, no cold starts, full control.

### Server layout (multi-project standard)

Pick a directory on your server (e.g. `/opt`) and follow this layout for
**every** project that adopts the pattern — siblings share the same shape:

```
/opt/
└── gogogo-fullstack-template/                  ← this project
    ├── bin/
    │   ├── gogogo-fullstack-template             ← current binary (chmod 755)
    │   └── gogogo-fullstack-template.previous    ← prior binary, kept for fast rollback
    ├── compose/
    │   └── docker-compose.prod.yml
    ├── env/
    │   └── .env                       ← non-secret env (DATABASE_URL, APP_URL, ...)
    ├── secrets/
    │   └── gogogo-fullstack-template.env         ← mode 600, regenerated every deploy from GH Secrets
    ├── data/
    │   └── pb_data/                    ← persistent volume, survives restarts
    ├── repo/                           ← git clone of this repo (for re-syncing on each deploy)
    ├── scripts/
    │   └── deploy-prod.sh             ← the on-server deploy runner
    └── README.md                       ← operator's guide (link to this section)

/opt/<other-project>/                  ← siblings follow the same shape
    ├── bin/
    ├── compose/
    ├── env/
    ├── secrets/
    └── data/
```

### First-time setup on the server

1. Install Docker + create a `deploy` user with SSH key access.
2. Add the box to your Tailscale tailnet.
3. Configure a Cloudflare Tunnel that routes your domain (e.g. `fullstack.example.com`)
   to the Tailscale hostname on port 8080.
4. Clone the repo at `/opt/gogogo-fullstack-template/repo/` and run `./repo/scripts/setup-server.sh`
   (we ship this in a follow-up; the manual steps are: `mkdir -p bin compose env secrets data/pb_data scripts`).
5. Add the GitHub Actions secrets (see `.github/workflows/deploy.yml` for the full list).

### After setup, every push to `master` deploys

The workflow at `.github/workflows/deploy.yml` runs on every push to `master` and:

1. Builds the project (lint + race tests + CSS build).
2. Builds the production Docker image (linux/amd64 scratch) in the GH Action runner.
3. SCPs the new binary to the server as `gogogo-fullstack-template.new` (atomic swap).
4. Writes the secrets file (`/opt/gogogo-fullstack-template/secrets/gogogo-fullstack-template.env`) with mode 600.
5. SSHes in and runs `scripts/deploy-prod.sh` which:
   - Atomically renames `gogogo-fullstack-template.new` → `gogogo-fullstack-template` and keeps the old binary as `.previous`.
   - Restarts the container via `docker compose -f docker-compose.prod.yml up -d`.
   - Waits up to 30s for `/health` to return 200.
6. Prints the new container status + last 20 log lines for confirmation.

Secrets are **never stored long-term on the server**: every deploy
re-renders `/opt/gogogo-fullstack-template/secrets/gogogo-fullstack-template.env` from GitHub
Actions secrets. The file is `chmod 600`, owned by the `deploy` user,
and overwritten on every run — there is no history of secrets on disk.

## Structure

```
cmd/web/
  main.go               # Entry point — only initializes and wires pieces    nats.go               # NATS JetStream bootstrap
    dagnats.go            # DagNats runtime bootstrap
config/                 # Per-environment config (dev/prod)
db/                     # PocketBase setup + seed
internal/
  secrets/              # age-decrypted secrets loader
  queue/                # goqite + SSE Hub + workers + retry + handler registry
    goqite.go           #   goqite setup, schema (goqite_schema.sql), graceful shutdown
    ssehub.go           #   register-before-enqueue, replay buffer, backpressure
    workers.go          #   worker pool with context cancellation
    retry.go            #   exponential backoff + jitter (retry-go v4)
    handlers.go         #   HandlerRegistry: job-type to handler dispatch
  nats/                 # NATS JetStream + embedded server
  dagnats/              # DagNats durable workflow client + onboarding JSON
  llm/                  # GoAI LLM SDK helpers
  datastar/             # Datastar rendering helpers
features/
  todo/                 # Working example: Todo MVC
    handlers/           #   HTTP + SSE handlers, onboarding
    components/         #   Templ components (layout, todo_list, todo_item, toast)
web/resources/          # Static assets (embedded JS)
router/                 # Routes registered on PocketBase
references/             # Reference documentation
docs/                   # Decision logs and guides
```

## Acknowledgements

This template was partially inspired by [northstar](https://github.com/zangster300/northstar) by Nicholas Zanghi — a Go + NATS + Datastar + Templ + DaisyUI application starter. northstar is released under the MIT License; if you build on this template, please also credit northstar.

## License, feedback

Licensed under the [MIT License](./LICENSE). This project is open to feedback, PRs, and adaptations. If something doesn't make sense, if the stack doesn't fit your problem, or if you have a better idea — open an issue.

---

Made with intent to be useful, not to be right. — feedback, PRs, and adaptations welcome.
