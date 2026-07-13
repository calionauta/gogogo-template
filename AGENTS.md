# gogogo-fullstack-template

> Full-stack Go web app template — back-end + front-end + DB + auth + LLM + deploy in one binary.

## Project Overview

Go template: Datastar + Templ + PocketBase + goqite + DagNats + NATS JetStream.
Module: `github.com/calionauta/gogogo-fullstack-template`

**Naming:** repo, module, binary, deploy dir (`/home/deploy/<APP_NAME>/`), container, and tunnel hostname all share the project name. Replace `gogogo-fullstack-template` everywhere when cloning.

**Unified build.** `go build ./cmd/web` or `make build` compiles **everything** — no build tags. Every feature (queue, workflows, realtime, whiteboard, onboarding) is always included. Opt out at runtime via env vars like `NATS_ENABLED=false`, `DAGNATS_ENABLED=false`.

## Stack (exact versions)

Go 1.26 | Templ v0.3.1020 | Datastar v1.2.2 | PocketBase v0.39.5 (ncruces/go-sqlite3) | TailwindCSS v4.1.13 + DaisyUI v5.6.15 | goqite v0.4.0 | retry-go v4 | DagNats v0.0.5 | NATS JetStream | age v1.3.1 | uuid v1.6.0

Skills: `cali-coding-go-standards` (code quality), `cali-code-navigation` (cymbal-first search). Install via `npx skills add .../cali-coding-go-standards`.

## Commands

| Command | Description |
|---------|-------------|
| `make dev` | Air live reload (gofumpt + vet + golangci-lint info) |
| `make build` | Unified build (everything included) |
| `make test` | Race tests (`-p 1` for DagNats engine stability) |
| `make templ` | Generate Templ |
| `make datastar-lint` | Lint `.templ` via datastar-lint (`-only-errors` keeps intentional custom attrs) |
| `make check` | **Gate**: fmt + datastar-lint + golangci-lint + vet + sizes + deadcode + race tests |
| `make ci-local` / `make signoff` | Local CI gate + gh-signoff stamp (see Local CI) |
| `make setup` | Blocking pre-commit + pre-push (pre-push adds `govulncheck`) |

## Don'ts

- NO HTMX/Alpine — use Datastar. NO `fmt.Sprintf` for HTML — use Templ.
- NO raw CSS class when a DaisyUI component exists. NO `log` — use `log/slog`.
- NO `modernc.org/sqlite` (driver is ncruces/go-sqlite3). NO removing goqite when adding JetStream; they solve different problems.
- NO manual `id` on PocketBase records (PK Max=15, `^[a-z0-9]+$`).
- NO Datastar `PatchElements` whose top-level element lacks `id` + `WithSelector` (client throws `PatchElementsNoTargetsFound`). Use `internal/datastar.RenderAndPatch` paired with a selector.
- NO real LLM in tests — inject a stub (`internal/llm/fakeserver` only inside `internal/llm/`).

## Architecture (concise)

```
cmd/web/        Entry point (PB + goqite + SSE Hub + DagNats + NATS)
config/         Env config
db/             PocketBase + collection seeds
internal/       {secrets,queue,nats,dagnats,llm,datastar,collab}
features/app/   AppContext (cross-cutting deps bundle)
features/todo/  Todo MVC (Datastar + DaisyUI + PB + SSE Hub + DagNats onboarding)
features/whiteboard/  Loro CRDT + Rough.js canvas + SSEHub + NATS sync
web/resources/  Embedded static assets
```

**Three complementary async layers:** `goqite` (jobs+SSE) · `dagnats` (durable workflows) · `JetStream` (cross-instance realtime). They coexist in the same binary; all three are always compiled.

**Routing (read before touching `router.Init`):** PocketBase `RouterGroup` compiles to stdlib `http.ServeMux` (Go 1.22+ subtree matching — `GET /` swallows unregistered subpaths). Register all routes DIRECTLY on `se.Router` inside the OnServe hook (nested `OnServe().BindFunc` never fires). App cookie is `gogogo_auth` (NOT `pb_auth`) — the two cookies are intentional: PocketBase keeps admin (`_superusers`) and regular users as SEPARATE auth namespaces, so sharing `pb_auth` clobbers the admin session in the same browser (known PB gotcha, issues #5050/#1780). Run the admin UI on a separate origin/port (`:8090/_/`) so even `pb_auth` never collides. Serve static assets via EXACT `/static/<file>` routes (PB catch-all shadows wildcards). Full routing war-stories: `docs/decisions.md`.

## Realtime transport decision

**Todo records** (create/toggle/delete) flow through **PocketBase realtime** — the realtime SSE lives at `/api/realtime`, is authenticated by the app's `LoadAuthFromCookie` middleware (reads `gogogo_auth`), and the collection's `ListRule`/`ViewRule` (`@request.auth.id != '' && owner = @request.auth.id`) make delivery **per-user scoped**. Each subscribed client re-fetches `/api/todos/fragment` and morphs `#todo-list` on a `todos` event. This is the mechanism for DB actions — do NOT add a parallel SSE-hub re-render for todo mutations.

**The SSE hub (`/api/todos/stream`)** is reserved for **ephemeral signals only**: the live clients count, LLM suggest feedback, and DagNats workflow progress. It also carries the **originating client's** synchronous patch on its own mutation POST. It does NOT broadcast record mutations to other clients.

**Whiteboard** uses **SSEHub + NATS** — shapes are dual-broadcast: in-process via the SSE Hub (same-process tabs) and over NATS via the SyncWorker (cross-instance convergence). Presence cursors use the same SSE Hub with exclude-origin fan-out. Clients are **offline-first**: Loro CRDT merges late/replayed ops on reconnect (outbox in `whiteboard.js`).

**NATS JetStream** is used by DagNats (workflow engine state), the whiteboard SyncWorker (cross-instance doc sync), and the optional desktop-edge Leaf Node. The todo broadcaster uses an in-memory fan-out by default (can be wired to JetStream for multi-instance deployments).

## Local CI (gh-signoff)

CI runs on push to `master` then deploys. Run the **same gate locally** to avoid broken pushes:

```bash
gh extension install basecamp/gh-signoff
make ci-local      # templ + golangci-lint + datastar-lint + css-check + race tests + build
make signoff       # ci-local + gh signoff -f
```

Uses golangci-lint (not standalone gofumpt) as the formatter gate — gofumpt can be a newer release than golangci-lint bundles, causing false positives. Signoff is **advisory** (push-to-master flow, not PR merge) — do NOT `gh signoff install`.

### Pre-push workflow: gate locally, then ask before pushing

Remote CI + deploy is slow and runs on every push to `master`. To save time, **always run the local gate first** and only push when it is green:

1. `make ci-local` (full local gate).
2. If it passes, **ask the user** (via `ask_user_question`) whether they want to push now or keep working.

Rationale: a developer often wants only a local green signal before continuing with more changes; pushing prematurely kicks off a slow remote run they may not need yet.

## Deploy

Push-to-`master` triggers `.github/workflows/deploy.yml` (Tailscale OIDC + Docker to single server). Server layout/deploy-user/secret tables: see `/skill:cali-ops-deploy-github-tailscale`. Two gotchas: (1) grant container write via `setfacl`/`chmod`, NEVER `chown` (non-root deploy user); (2) never `scp` into the server's repo clone — `git pull --ff-only` aborts. Scratch image healthcheck: `CMD [\"/app\",\"health\"]` (no `wget`/`curl`/`CMD-SHELL`).

## DaisyUI

ALL HTML UI uses DaisyUI components (read https://daisyui.com/llms.txt). Load `/static/app.min.css` (built by `npm run build`, regenerated in Dockerfile). NEVER `daisyui.min.css` (v4 relic, breaks v5 markup).

## Testing

Temp-dir PocketBase + Bootstrap + real SQLite; `httptest.NewServer` over a real router; assert against DB. LLM fakes via `internal/llm/fakeserver` (transport) or injected stubs (business logic). `go test -race -p 1 ./...` (serialized packages for DagNats engine stability).
