# Architectural Decisions

## Why goqite (Not NATS JetStream) as Default Task Queue

goqite is a SQLite-backed queue that runs in-process. Zero external dependencies.
- ✅ Fire-and-forget jobs with SSE streaming via Hub
- ✅ No network calls, no broker process
- ✅ ~18.5k msgs/s — enough for LLM calls, email, etc.

JetStream is available as an **additional** layer for multi-user real-time, when needed.

## Why DagNats for Durable Workflows

[DagNats](https://github.com/danmestas/dagnats) is a DAG-based durable workflow engine built on NATS JetStream. Workflows are **declarative JSON** (not Go code): the workflow references task *names* (strings), not Go symbols, so renaming a Go handler never orphans an in-flight run. Each step's result is recorded in the event-sourced history; on crash, the workflow resumes from the last completed step.
- ✅ Multi-step transactions that survive restarts
- ✅ Native in-step suspend via `WaitForSignal` (the engine blocks a step until an external signal arrives)
- ✅ Step retries with exponential backoff
- ✅ Scheduling (cron), human-in-the-loop approvals, sub-workflows, agent loops
- ✅ Runs as a library (the `server` package boots an embedded NATS + orchestrator + REST API + console) — no separate service to operate

DagNats is opt-in via the `dagnats` build tag. It boots an embedded NATS on the conventional port `:4222` and exposes its REST API + console on `DAGNATS_HTTP_ADDR` (default `127.0.0.1:8090`), separate from the app port. **Single-NATS convention:** under `-tags "jetstream dagnats"`, the realtime `TodoBroadcaster` does NOT start its own NATS — it connects to the one DagNats already owns on `:4222` (see `cmd/web/nats.go` → `ConnectExisting`). One NATS process, two consumers (DagNats workflows + JetStream realtime). Building `-tags dagnats` alone keeps the in-memory broadcaster (single-instance), since there is no `jetstream` tag to provide the JetStream-backed one.

## Why Three Async Layers (Complementary, Not Alternatives)

| Layer | Problem It Solves | When You Need It |
|-------|-------------------|------------------|
| goqite | "I need to run background tasks and notify the user" | Always (default) |
| dagnats | "I need N steps that survive a crash" | Complex onboarding, pipelines (opt-in build tag) |
| NATS JetStream | "Multiple users need to see the same live state" | Whiteboard, presence, shared UI (opt-in build tag) |

You can have all three in the same binary. They do not conflict.

## Why PocketBase (Not Plain SQLite)

PocketBase embeds as a Go library and provides:
- ✅ Built-in auth (OTP, OAuth2, JWT)
- ✅ Automatic REST API for collections
- ✅ Realtime subscriptions
- ✅ Admin dashboard
- ✅ File storage

Plain SQLite is available as an escape hatch when PocketBase is too opinionated.

## Why age + ~/.secrets/ (Not Doppler/Vault)

For 1-2 developer teams with <20 secrets:
- ✅ Zero external services
- ✅ Single binary (age is static-linked)
- ✅ No cloud dependency
- ✅ Simple mental model

Move to Doppler/Vault when the team grows or secrets exceed 20.

## v0.18.0 — Offline todo add + CI flake + CSS staleness (post-mortem)

Three issues shipped together in v0.18.0 (`4741055`). Each taught a separate lesson; together they form the "testing discipline" section in `AGENTS.md`.

### 1. Offline todo add button stuck in loading

**Symptom (live, gogogo.calionauta.com):** click Add while offline → button enters loading state and never returns. No banner. No feedback.

**Investigation:**
- `diff <(curl https://gogogo.calionauta.com/static/sw.js) <(repo web/resources/static/sw.js)` → byte-identical. Deploy was current (v0.17.0). Not a stale build.
- `grep` on `features/todo/components/todo_list.templ` → `createForm` does `if (!$loading) { $loading = true; @post('/api/todos?clientID=...', {contentType: 'form'}); }`. `$loading` is reset only by the server's HTML fragment response.
- Read `web/resources/static/sw.js` → on offline POST, `networkFirstWithQueue` queues in IndexedDB and returns `new Response(JSON.stringify({queued:true,...}), {status: 202})`. Datastar's `@post` sees a 2xx → resolves the promise → but the body is JSON, not an HTML fragment, so it patches nothing → `$loading` stays `true` forever → button stuck.

**Why no banner?** `OfflineBanner` should have shown via either the SW `sync-error` message or `navigator.onLine === false`. Hard to tell from the report whether the user actually saw it (browsers vary on the offline toggle's effect on `navigator.onLine`), but the primary symptom was the stuck button.

**Fix (`4741055`):**
- `internal/components/offline_banner.templ`: when the SW posts `sync-error`, dispatch `window.dispatchEvent(new CustomEvent('gogogo:queued'))`. Inline JS — unavoidable for SW bridge.
- `features/todo/components/todo_list.templ`: on the signals root, `data-on:gogogo:queued__window="$loading = false"`. Pure Datastar expression, zero JS.

**Design note:** the bridge stays inline (locality of behavior) because Datastar expressions can't subscribe to SW `postMessage`. The consumer is fully declarative — no per-form listener to add. Any future form that flips its own `$loading` gets the reset for free by mounting the OfflineBanner in its layout.

### 2. TestCrudConsumerCreate "flaky" — actually a Bootstrap panic

**Symptom:** `make test` failed ~every run with `--- FAIL: TestCrudConsumerCreate (0.01s)` in `internal/nats`. Isolation passes (`go test -run TestCrudConsumerCreate ./internal/nats/`).

**Investigation:**
- `make test > /tmp/suite.out 2>&1` then `grep -B 2 -A 15 'FAIL: TestCrudConsumerCreate' /tmp/suite.out` → revealed `panic: DBConnect config option must be set when the no_default_driver tag is used!` from `pocketbase/core.DefaultDBConnect`.
- Root cause: `Makefile`'s `go test $(TAGS) ...` lines forced `-tags no_default_driver` on every test binary. That tag (used legitimately by the shipped binary to exclude PB's bundled `modernc/sqlite` for size) requires every `app.Bootstrap()` to supply a `DBConnect`. Our tests use the default modernc driver, so Bootstrap panicked on first PB init.
- 0.01s timing + intermittent appearance in full suite (but not isolation) was the giveaway that this was a **build-config error masquerading as a race** — the consumer `t.Logf("consumer Run exited: connection closed")` from a previous test's goroutine was a red herring.

**Fix:** `Makefile`'s three `go test` recipes (line 44, 115, 153) no longer pass `$(TAGS)`. Build recipes keep it — the shipped binary still excludes modernc via `cmd/web`'s DBConnect (ncruces).

**Bonus hardening:** `internal/nats/embedded.go` `StartEmbedded` now nils `NS/NC/JS` on entry and `Stop` nils on exit. Defensive against any future cross-package leak (the package globals had no zero-reset, so a partially-torn-down state could be inherited).

### 3. CSS bundle silently stale

**Symptom:** `make ci-local` failed at `css-check`: "❌ CSS out of date. Run `make css` and re-commit." Even on a clean stash (no working changes). `git show HEAD:web/resources/static/app.min.css` → 272,022 bytes; `make css` produced 176,689 bytes (95 KB smaller, 3,229 lines deleted).

**Investigation:**
- `npm ci` is deterministic (lockfile pinned), so the build wasn't pulling newer deps between commits.
- Diff of utility classes: the committed bundle had `alert-*`, `badge-accent`, `btn-circle`, `bg-base-100`, etc. that no `.templ` file currently uses. Someone simplified/removed features earlier and never re-ran `make css`.
- `css-check` passed in CI by inertia: it runs `git diff --quiet --exit-code web/resources/static/app.min.css` after `make css`. When nobody touched the file, working tree == HEAD == checked-in stale bundle → no diff → "passes." The check only fires when something actually rebuilds.

**Fix (`c24d3f3`):** rebuild the CSS from current `.templ` sources, commit the smaller bundle. `css-check` now passes legitimately.

**Rule added to AGENTS.md:** any change to a `.templ` file must be followed by `make templ && make css`. The pre-commit hook already runs `make check` (which includes `css-check`), but the hook only fires when staged `.templ` files change; partial rebuilds (only `_templ.go`, not `app.min.css`) slip past it.

### Lessons

1. **Build tags are not test tags by default.** Tags that change lib behavior (drivers, feature gates) for the binary must be explicitly excluded from `go test`, OR every test bootstrap must supply the equivalent setup. Failure mode: panic with a confusing message, easy to misread as a race.
2. **Don't blame flake for a 0.01s panic.** Test ordering can mask real errors (the consumer `Run` `t.Logf` was a red herring; the real failure was earlier, in `Bootstrap`). Always dump the full panic stack on suite failure, not just `FAIL|ok` lines.
3. **The cheapest "is the fix live?" test is a byte diff of an embedded asset.** `diff <(curl /static/<asset>) <(repo <asset>)` — if it matches, the running binary embeds the latest source. Took seconds to confirm v0.17.0 was deployed.
4. **Stale committed artifacts (CSS, generated files) can pass CI by inertia** when no rebuild is triggered. Make the rebuild part of the change that introduces the source drift, or have a guard that fails when generated files are older than their sources.
5. **Locality of behavior for the bridge, declarative for the consumer.** The OfflineBanner script stays inline (SW postMessage isn't reachable from Datastar expressions); the consumer side (`data-on:gogogo:queued__window`) is pure Datastar. No per-form glue code; future features inherit the fix by mounting the banner.
