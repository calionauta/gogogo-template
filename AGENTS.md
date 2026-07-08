# gogogo-template

## Project Overview

Go web application template with Datastar + Templ + PocketBase + goqite + Turbine + NATS JetStream.
Module: `github.com/calionauta/gogogo-template`

## Stack

Go 1.26 | Templ v0.3.1020 | Datastar v1.2.2 (datastar-go) | PocketBase v0.39.5 (embedded, ncruces/go-sqlite3) | TailwindCSS v4.1.13 (CLI build) + DaisyUI v5.6.15 | goqite v0.4.0 | retry-go v4 | Turbine v0.3.0 (opt-in, build tag) | NATS JetStream (opt-in, build tag) | age v1.3.1 | uuid v1.6.0

## Skills

```bash
npx skills add https://github.com/calionauta/agent-sync-public/tree/main/skills/cali-coding-go-standards --yes
npx skills add https://github.com/calionauta/agent-sync-public/tree/main/skills/cali-code-navigation --yes
```

| Skill | When |
|-------|------|
| `cali-coding-go-standards` | Code quality: KISS, DRY, file size, functions, `slog`, error handling, tests, lints |
| `cali-code-navigation` | Code search & impact: cymbal-first, fff fallback, sem diff for refactors |

For stack-specific patterns, see `docs/` and `references/` (Skill assets).

## DaisyUI Components

For ALL HTML UI, use DaisyUI components. Read https://daisyui.com/llms.txt before building any UI.

## Commands

| Command | Description |
|---------|-------------|
| `make dev` | Live reload with Air (post-build: gofumpt + go vet + golangci-lint info) |
| `make build` / `build-jetstream` / `build-turbine` / `build-all` | Build binary with optional tags |
| `make test` / `test-jetstream` / `test-turbine` | Run tests with race detector |
| `make templ` | Generate Templ components |
| `make fmt` | Check formatting (gofumpt + goimports) |
| `make datastar-lint` | Lint `.templ` / `.html` via [datastar-lint](https://github.com/calionauta/datastar-lint) |
| `make check` | **Full gate**: fmt + datastar-lint + golangci-lint (no `--fast`) + vet + sizes + deadcode + race tests |
| `make setup` | Install blocking pre-commit + pre-push hooks |
| `bin/datastar-lint` | Wrapper that installs and runs datastar-lint (also wired into `make check`) |

## Don'ts

- Do NOT use HTMX/Alpine.js — use Datastar
- Do NOT use `fmt.Sprintf` for HTML — use Templ
- Do NOT remove goqite when adding JetStream (they are complementary)
- Do NOT use modernc.org/sqlite — driver is ncruces/go-sqlite3
- Do NOT use raw CSS class names when a DaisyUI component exists
- Do NOT use `log` package — use `log/slog` (stdlib since Go 1.21)
- Do NOT manually set the `id` field on a PocketBase record (PK Max=15, Pattern=^[a-z0-9]+$)
- Do NOT call the real LLM in tests — inject an `LLMClient` interface and use a fake

## Architecture

```
cmd/web/                  Entry point (PB + goqite + SSE Hub). Builds: jetstream | turbine (opt-in).
config/                   Env-based config
db/                       PocketBase setup + ensureTodosCollection seed
internal/{secrets,queue,nats,workflow,llm,datastar}/
features/app/             Application core: AppContext + cross-cutting middleware
features/todo/            Example: Todo MVC (Datastar + DaisyUI + PocketBase + SSE Hub)
web/resources/            Static assets (embedded)
router/                   Route registration on PocketBase
docs/                     Decision logs and guides
```

Three async layers (complementary, not alternatives):
`goqite` → background jobs + SSE; `turbine` → durable workflows; `JetStream` → multi-user real-time.

## Build tags (`goqite` is core, the others are opt-in)

The default build (`go build ./cmd/web`) is the recommended starting point for almost every project. It ships the full UI, the queue, the SSE Hub, the LLM client, the auth feature, and the demo Todo app. The only async layer that's gated is `goqite` itself being **always on** — the others are opt-in heavyweight features:

| Build tag | Binary size impact | What you get | Where it's wired |
|---|---|---|---|
| _(default)_ | baseline | `goqite` queue + `InMemoryBroadcaster` (single-process cross-client SSE) + demo Todo app + auth + LLM | `router/router.go` |
| `-tags jetstream` | +9 MB | Above + an embedded NATS server + durable `TODOS` JetStream stream for multi-instance realtime | `cmd/web/nats.go` (`//go:build jetstream`) + `cmd/web/nats_noop.go` (`//go:build !jetstream`) |
| `-tags turbine` | +2 MB | Above + durable multi-step workflows (e.g. `WelcomeOnboarding`) and the Turbine executor in PocketBase SQLite | `cmd/web/turbine.go` + `cmd/web/turbine_noop.go` |
| `-tags "jetstream turbine"` | +11 MB | Both opt-ins stacked | sum of the above |

The build-tag pattern is **file-level** in `cmd/web/`. Each tag has a noop stub (e.g. `turbine_noop.go`) so the default build never references the heavy deps. The router checks `cfg.NATS.Enabled` / `cfg.Turbine` runtime flags to decide whether to call the wiring functions; feature handlers (`features/todo/handlers/admin.go`, `.../onboarding.go`) gate their behaviour on `h.llm != nil && h.llm.Configured()` and the cfg flags, so routes 404 cleanly when features are off rather than crash.

When adding a new feature with an optional dependency, follow the same shape: `internal/feature/<name>.go` (real impl) + `internal/feature/<name>_noop.go` (default build stub) + a `cfg.<Name>.Enabled` flag. See `docs/decisions.md` for the rationale.

## Cross-cutting application core

`features/app/` provides `AppContext` — a single struct that bundles
the cross-cutting dependencies every feature might need (queue, LLM
client, config). The template itself uses it lightly (mainly for
`LogStartupSummary`), but downstream projects that grow to multiple
features can wire their handlers to take `*AppContext` instead of
assembling (queue, llm, broadcaster, ...) individually. See the
package doc in `features/app/app.go` for the full rationale.

## Quality Gate

Run `make check` after each significant edit. The pre-commit hook (`make setup`) is blocking on the same gate. Pre-push adds `govulncheck`. See `docs/decisions.md` for the why.

## Realtime (todo sharing across clients)

Cross-client todo mutations go through `nats.TodoBroadcaster` (wired in `router.Init`). See the "Build tags" table above for the runtime vs JetStream trade-off. The `features/todo/handlers/todo.go` `broadcastTodo()` helper is the single chokepoint: it goes through `h.broadcaster` (the broadcaster field) and is silently skipped if `nil`. So a feature handler doesn't need to know whether the deployment is single-instance or multi-instance — it just calls `broadcastTodo()`.

## LLM Integration (GoAI)

`internal/llm` wraps GoAI behind an injectable interface. Tests must NOT call the real provider — inject a fake (or VCR replay). Streaming modeled as an iterator so backpressure/cancel are testable.

## Testing

Pattern: temp-dir PocketBase + Bootstrap + real SQLite (no mocks), `httptest.NewServer` over a real router, assert against the database.
