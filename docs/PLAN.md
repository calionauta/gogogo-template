# PLAN.md — multi-session roadmap for gogogo-fullstack-template

> Living document. Each numbered section is a **future session**.
> Read "What we'll do" before opening the session to know the starting
> point; "Exit criteria" tells you when the session is done.
>
> Update this file at the end of each session, not during. The session
> that closes work also commits the updated PLAN.md.

---

## 1. Wails v3 — desktop + mobile builds

### What we'll do

Add an optional second frontend target to the same project: a native
desktop app (Windows/macOS/Linux) and a mobile app (Android/iOS),
both built on top of the same Go business logic that already powers
the web app. Wails v3 wraps the existing handlers in a webview and
binds Go methods to JS — we add two thin entry points (`cmd/desktop/`,
`cmd/mobile/`) without rewriting the web stack.

CI strategy: GitHub Actions runners build all platform artifacts.
A forker who can push a tag gets a release with `.dmg`, `.msi`,
`.deb`, `.AppImage`, `.apk`, `.ipa` attached — zero local
toolchains required.

### Exit criteria

- [ ] `make desktop` produces a runnable macOS `.app` locally
- [ ] `make android` produces a runnable `.apk`
- [ ] `.github/workflows/build-platforms.yml` runs on every push and
      uploads 6 platform artifacts
- [ ] `cmd/desktop/main.go` and `cmd/mobile/main.go` are < 200 lines
      each, reusing the existing router/handlers
- [ ] README has a "Desktop & Mobile" section with screenshot

---

## 2. Bug #6 — RESOLVED in commit 23d69b5 ✅

**Status**: fixed. No further action.

The 303 redirect loop on `/` and `/login` was caused by registering
routes inside `app.OnServe().BindFunc(...)` from inside another
`OnServe` handler. PocketBase's `Hook.Trigger` snapshots registered
handlers before any of them run, so nested `BindFunc`s are silently
a no-op. Fix: `RegisterAuth` now takes `*core.ServeEvent` and wires
routes directly on `se.Router` inside the existing top-level
router.OnServe hook. See AGENTS.md §"Wiring HTTP routes" for the
full rule and `http://localhost:8080/<path>` curl recipes.

**Follow-up (2026-07-08):** the first superuser for
`gogogo.calionauta.com` was created via the install UI this session,
and the app was verified working (after Bug #8). The site is
reachable at `gogogo.calionauta.com` (Cloudflare Tunnel terminates
at the server). Remaining operator action is **Cloudflare Access**
(step 4): put an Access policy / login wall in front of
`gogogo.calionauta.com` so the PocketBase superuser / install UI
can't be brute-forced from the public internet. See the step-4 notes
in the session handoff.

---

## 3. Local-first repo CI as reusable workflow

The current `make check` / `make gate` works locally. Next session
extracts this into a reusable GitHub Actions workflow
(`.github/workflows/lint-test.yml`) that downstream forks can
inherit with zero config.

---

## 4. Multi-project promotion

If `gogogo-fullstack-template` becomes the foundation for `stelow`
or `datastar-lint`, the canonical `/var/lib/<name>/data/` bind-mount
layout is the shared convention. Extract `scripts/deploy-prod.sh`
into a separate `gogogo-deploy` repo so the three projects share
the same deploy runner.

---

## 5. Pending cleanups

- [x] `/home/deploy/gogogo-template/` (legacy deploy dir on the
      server) removed 2026-07-08 — `gogogo-fullstack-template` is
      stable (superuser created, app verified working).
- [x] First superuser for `gogogo.calionauta.com` created via the
      install UI 2026-07-08 (see Bug #6 status).
- [ ] **Cloudflare Access (step 4):** put an Access policy / login
      wall in front of `gogogo.calionauta.com` (same pattern as
      `server-treinador.calionauta.com`) so the PB superuser / install
      UI can't be brute-forced. See session handoff notes.
- [ ] Old `gogogo.calionauta.com` install link JWTs have all expired
      — current install link printed by `docker logs` on every
      restart; no longer needed now that the superuser exists.

---

## 6. On-prem deploy permission gotcha (real bug, real fix)

**Symptom** (this session, 2026-07-08): PocketBase container fails
on every restart with `sqlite3: unable to open database file:
permission denied`. The data dir is on a Docker named volume
(`gogogo-fullstack-template-data:/var/lib/.../data`). Docker
creates new volumes with `root:root` ownership, but the container
runs as `USER 65532:65532`. Result: crash loop on first deploy to a
fresh server.

**Root cause**: Docker volume permissions are not negotiated against
the container's `USER`. Bind mounts work because `deploy-prod.sh`
can `chown 65532:65532` the host directory before starting.

**Fix shipped in commit `2c8a1f6`** (this session):
- `deploy/docker-compose.prod.yml`: switched to bind mount
  `/var/lib/${APP_NAME}/data:/var/lib/${APP_NAME}/data`
- `scripts/deploy-prod.sh`: creates `/var/lib/${APP_NAME}/data` with
  `chown 65532:65532` before bringing the container up

**Lesson for other projects**: prefer bind mounts over named volumes
when the container runs as a non-root UID. Bind mounts give the
operator full file-system access for inspection, backup, and
permission repair. Named volumes are appropriate only when the
container runs as root (rare for production).

---

## 7. Bug #7 — deploy permission gotcha: `chown 65532:65532` fails as non-root (RESOLVED 2026-07-08)

**Symptom**: `scripts/deploy-prod.sh` aborts at
`chown 65532:65532 /var/lib/<APP>/data` — the `deploy` user is
non-root and **cannot chown to an arbitrary UID**. The GitHub Action
run goes red, and if the container is later started by hand the data
dir is wrong-owned → `sqlite3: permission denied` crash loop. (This
is what broke run `28975753469`.)

**Why the earlier "fix" (commit `2c8a1f6`) was incomplete**: it
switched to a bind mount but *still* ran `chown 65532:65532`, which
the non-root `deploy` user cannot do. That chown only ever worked
when someone manually `sudo`-ed it once on the server.

**Real fix (commit `8ff3f12`, this session)**:
- `scripts/deploy-prod.sh`: data dir moved to
  `/home/deploy/<APP>/data` (deploy-owned; `/var/lib` is root-owned
  and untouchable). Write access for container uid 65532 granted via
  `setfacl -R -m u:65532:rwx -d -m u:65532:rwx` (or `chmod -R 0777`
  fallback). No `chown` to 65532 anywhere.
- `deploy/docker-compose.prod.yml`: host source is now
  `/home/deploy/<APP>/data`; container target stays
  `/var/lib/<APP>/data` (so the in-container `DATA_DIR` env is
  unchanged).
- Recursive + default ACL matters: the workflow's "Ensure layout"
  step pre-creates `data/pb_data` as the deploy user, so the default
  ACL lets uid 65532 still write into it.

**Second, silent failure mode**: the GitHub Action does
`git -C <repo> pull --ff-only` in "Ensure layout". If you `scp`
edited files into the server's repo clone (`/home/deploy/<APP>/repo`)
to test a deploy, the dirty tree makes `git pull` abort and the whole
deploy fails. **Always push to `master`; never scp into the repo
clone.** (This bit us during the 2026-07-08 session and was fixed by
`git -C repo checkout -- . && reset --hard origin/master` then
re-running the workflow.)

See AGENTS.md §Deploy and §"Wiring HTTP routes".

---

## 8. Bug #8 — static assets 303-redirect + auth middleware never wired (RESOLVED 2026-07-08)

**Symptom**: app loads with no styling; console shows
`Refused to apply style from '…/login' because its MIME type
('text/html') is not a supported stylesheet MIME type` and
`Failed to load module script … MIME type of "text/html"`. The
sign-in button looked dead.

**Root cause (two independent bugs)**:
1. PocketBase registers a catch-all dashboard route
   `e.Router.GET("/{path...}", apis.Static(...))`
   (`apis/base.go`) that requires superuser and redirects
   unauthenticated requests to `/login`. That catch-all SHADOWS any
   wildcard route we register (e.g. `/static/*`) — the handler is
   never reached, so every `/static/*` request is 303-redirected to
   `/login` and the browser gets HTML where it expected CSS/JS.
2. `LoadAuthFromCookie` was only wired in the *test* harness
   (`fixture_test.go`), never in production. So `e.Auth` was always
   nil → every authed page (`/`) bounced to `/login` even with a
   valid `pb_auth` cookie. (This was the real reason the install
   debug log showed `e.Auth` nil — not a stale JWT.)

**Fix (this session, same commit wave as Bug #7)**:
- `router/router.go`: serve embedded static assets via one EXACT
  route per file (`fs.WalkDir` over `resources.StaticFS()`, register
  `/static/<file>` exactly). Exact routes beat PB's catch-all.
- `web/resources/resources.go`: `StaticFS()` now returns `fs.FS`
  (was `http.FileSystem`).
- `router/router.go`: wire
  `se.Router.BindFunc(auth.LoadAuthFromCookie)` as a global
  middleware so `e.Auth` is populated on every request.
- Removed the temporary debug `BindFunc` from
  `features/auth/wiring.go`.

**Lesson**: never register a wildcard route to serve unauthenticated
files — PB's `{path...}` catch-all owns every unmatched path. Use
exact routes, or serve from PB's own `pb_public` dir. And any
middleware a test harness wires (like `LoadAuthFromCookie`) MUST also
be wired in production, or `e.Auth` is silently nil everywhere.

---

## Notes for whoever picks this up

- The deploy pipeline is **Pattern B** (shell key, image built on
  server) — see `~/.agents/skills/cali-ops-deploy-github-tailscale/`
  for the two patterns and when to use each.
- The redirect-loop and `http.ServeMux` subtree-matching gotchas are
  documented in AGENTS.md §"Wiring HTTP routes". The static-asset
  catch-all + unwired-auth-middleware gotchas are Bug #8.
- The privacy filter-repo pass is documented in the git history;
  backup mirror at `~/backups/gogogo-template-*.git/`.
- For PocketBase permissions, the rule is: **data dir lives under
  `/home/deploy/<APP>/data`** (deploy-owned), and
  `scripts/deploy-prod.sh` grants container uid 65532 rwx via
  `setfacl -R -m u:65532:rwx -d -m u:65532:rwx` (or `chmod -R 0777`).
  Never `chown 65532:65532` (the deploy user is non-root and it
  fails). `deploy/docker-compose.prod.yml` bind-mounts
  `/home/deploy/<APP>/data` → `/var/lib/<APP>/data`.