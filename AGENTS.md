# cali-go-stack

## Project Overview

Go web application template with Datastar + Templ + PocketBase + goqite + Turbine + NATS JetStream.
Module: `github.com/calionauta/cali-go-stack`

## Stack

Go 1.26 | Templ v0.3.1020 | Datastar v1.2.2 (datastar-go) | PocketBase v0.39.5 (embedded, ncruces/go-sqlite3) | DaisyUI v5.6.13 + TailwindCSS | goqite v0.4.0 | retry-go v4 | Turbine v0.3.0 (opt-in, build tag) | NATS JetStream (opt-in, build tag) | age v1.3.1 | uuid v1.6.0

## Skills

Install via `npx skills add`:

```bash
npx skills add https://github.com/calionauta/agent-sync-public/tree/main/skills/cali-coding-go-standards --yes
npx skills add https://github.com/calionauta/agent-sync-public/tree/main/skills/cali-code-navigation --yes
```

| Skill | When |
|-------|------|
| `cali-coding-go-standards` | Code quality: KISS, DRY, file size, functions, `slog`, error handling, tests, lints |
| `cali-code-navigation` | Code search & impact analysis: cymbal-first, fff fallback, sem diff for refactors. Especially useful on this template because the codebase mixes Go (server), Templ (HTML), and Datastar expressions — cymbal is Go-aware, fff handles the rest |

For stack-specific patterns (Datastar, Templ, DaisyUI, goqite, Turbine, JetStream), see the project's `references/` directory and `docs/` directory.

## DaisyUI Components

Before building any UI, read the full DaisyUI component catalog:
- https://daisyui.com/llms.txt

Use DaisyUI components for ALL HTML UI. Prefer: `table`, `input`, `btn`, `badge`, `card`, `modal`, `toast`, `loading`, `collapse`, `join`, `kbd`, `select`, `toggle`, `range`, `progress`, `chat`, `diff`, `stat`, `timeline`.

## Commands

| Command | Description |
|---------|-------------|
| `make dev` | Live reload with Air |
| `make build` | Build binary |
| `make build-jetstream` | Build with NATS JetStream |
| `make build-turbine` | Build with Turbine workflows |
| `make build-all` | Build with JetStream + Turbine |
| `make test` | Run tests with race detector |
| `make test-jetstream` | Run tests with JetStream tag |
| `make test-turbine` | Run tests with Turbine tag |
| `make templ` | Generate Templ components |
| `make fmt` | Check formatting (gofumpt + goimports) |
| `make datastar-lint` | Lint `.templ` / `.html` for Datastar anti-patterns (installs [datastar-lint](https://github.com/calionauta/datastar-lint)) |
| `make check` | **Full quality gate: fmt + datastar-lint + golangci-lint (no --fast) + vet + sizes + deadcode + race tests** |
| `make setup` | Install the blocking pre-commit hook (runs the same gate) |
| `golangci-lint run ./...` | Run linter |
| `bin/datastar-lint` | Wrapper that installs and runs [datastar-lint](https://github.com/calionauta/datastar-lint) (also wired into `make check`) |

## Don'ts

- Do NOT use HTMX/Alpine.js — use Datastar
- Do NOT use fmt.Sprintf for HTML — use Templ
- Do NOT remove goqite when adding JetStream (they are complementary)
- Do NOT use modernc.org/sqlite — driver is ncruces/go-sqlite3
- Do NOT use raw CSS class names when a DaisyUI component exists — check `daisyui.com/llms.txt`
- Do NOT use `log` package — use `slog` (`log/slog`, stdlib since Go 1.21)
- Do NOT set the `id` field manually on a PocketBase record — the collection's PK has Max=15 and Pattern=^[a-z0-9]+$, let PocketBase auto-generate
- Do NOT call the real LLM in tests — inject an interface (`LLMClient`) and use a fake/in-memory fixture, or replay VCR-style recorded responses. Token cost must be zero in CI.
- Do NOT accumulate edits without running the gate. Run `make check` after EVERY significant edit (every function/method added or changed), not just before commit. The full gate is cheap and catches errcheck/`slog`/format/lint issues immediately instead of as a 59-issue pileup at the end.
- Do NOT broadcast todo mutations via PocketBase's native WebSocket subscribers for cross-client realtime — use the JetStream broadcaster (`-tags jetstream`) or the in-memory `SSEHub.Broadcast` (default). PocketBase's subscriber transport is not the project's realtime layer.

## Architecture

```
cmd/web/main.go          → Entry point (PB embedded + goqite + SSE Hub)
  -tags jetstream        → Also starts embedded NATS server + JetStream
  -tags turbine          → Also starts Turbine durable workflow runtime
config/                  → Env-based config (dev/prod build tags)
db/                      → PocketBase setup + ensureTodosCollection seed
internal/
  secrets/               → age-decrypt loader
  queue/                 → goqite + SSE Hub + workers + retry (schema in goqite_schema.sql)
  nats/                  → NATS JetStream (build-tag gated)
  workflow/              → Turbine durable workflows (build-tag gated)
  llm/                   → GoAI client
  datastar/              → Datastar render helpers
features/
  app/                   → Application core
  todo/                  → Example: Todo MVC (Datastar + DaisyUI + PocketBase + SSE Hub)
web/resources/           → Static assets (embedded)
router/                  → Route registration on PocketBase

Three async layers (complementary, not alternatives):
  goqite    → background jobs (fire-and-forget, SSE streaming)
  turbine   → durable workflows (multi-step, crash recovery)
  JetStream → multi-user real-time (KV, streams, presence)
```

## Quality Gate

The single gate is `make check`. It runs, in order: `fmt` (gofumpt + goimports),
`datastar-lint` (`.templ` anti-patterns), `golangci-lint run ./...` (full, **no
`--fast`**), `go vet ./...`, `check-sizes` (per-file ≤ 250 lines for app code),
`deadcode` (app packages only), and `go test -race ./...`. The pre-commit hook
(`make setup`) enforces the same gate and is **blocking**.

**Rule:** run `make check` after each significant edit. Do not batch dozens of
edits and check once at the end — that is how 59 lint issues accumulated.

## Realtime (todo sharing across clients)

Todo mutations are shared across all connected clients through the
`nats.TodoBroadcaster` interface, wired in `router.Init`:

- **default build** → `InMemoryBroadcaster` fans out via `SSEHub.Broadcast` (single-instance, all tabs on this server).
- **`-tags jetstream`** → `JetStreamBroadcaster` publishes to a durable `TODOS` stream; a per-process subscriber re-emits to the SSE Hub so every tab on any instance receives the update.

This is the correct choice over PocketBase's native WebSocket subscribers:
the project already owns an SSE Hub transport, and JetStream complements
(not replaces) goqite — goqite still owns the async job + per-client toast path.
The `broadcastTodo` call in `handlers/todo.go` is a no-op when no broadcaster
is set.

## LLM Integration (GoAI)

`internal/llm` wraps GoAI behind an injectable client. Tests must NOT call the
real provider — define an `LLMClient` interface and inject a fake that returns
recorded/replayed responses. See `cali-coding-go-standards` for the e2e-LLM
playbook (fake provider, VCR replay of recorded responses, golden JSON files,
contract tests gated on `OPENAI_API_KEY`, streaming modeled as an iterator).

## Testing

Integration tests live next to the code they exercise. Pattern (from cali-coding-go-standards):
- temp-dir PocketBase + Bootstrap + real SQLite (no mocks)
- httptest.NewServer over a real router
- assert against the database, not the rendered HTML

Coverage in this repo:
- `features/todo/sse_test.go` — HTTP → queue → worker → SSE toast pipeline
- `features/todo/retry_test.go` — SSE-aware retry feedback (parses `lastRetry` signals)
- `features/todo/onboarding_test.go` (build `turbine`) — `POST /api/onboarding/start` creates 3 todos via durable Turbine steps
- `internal/queue/queue_test.go` — Enqueue/Receive, worker dispatch, retry, not-found handler, idempotent Close
- `internal/workflow/turbine_test.go` (build `turbine`) — durable workflow end-to-end
- `features/todo/ssehub_test.go`, `todo_test.go` — SSE Hub + retry config + registry unit tests

See `features/todo/sse_test.go` for the canonical SSE-pipeline example.