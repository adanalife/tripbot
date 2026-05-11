# Changelog

<!-- markdownlint-disable MD024 -->
<!-- Duplicate sibling headings (Cleanup, OBS, CI, etc.) are intentional — same section names recur per release entry. Keep-a-Changelog convention. -->

All notable changes to TripBot. Format follows [Keep a Changelog](https://keepachangelog.com); versioning follows [Semantic Versioning](https://semver.org).

## [v2.3.2] — 2026-05-11

Patch release. Pre-bakes the OBS arm64 CEF compile into a base image (skipping ~25 min off every OBS PR), fixes four workflow triggers that pointed at a non-existent `main` branch (restoring Coveralls base-build uploads on `develop` and adding `pull_request` scanning to CodeQL), normalizes the OBS scene's seven `browser_source` on-screens to a clean thirds layout (fixes longstanding middle-text clipping), plus a small CI/env hygiene sweep.

### CI

- **OBS arm64 base image (`adanalife/obs-cef-base:arm64-*`).** New `infra/docker/obs/Dockerfile.arm64-base` compiles OBS-from-source against the aarch64 CEF tarball and ships a slim image carrying just `/opt/obs-install/`. New `obs-base.yml` workflow builds and pushes the base on demand. `infra/docker/obs/Dockerfile.arm64` now `FROM`s the base, so the arm64 leg of `obs.yml` drops from ~30 min to ~2 min. Bumping OBS/CEF becomes: edit the base Dockerfile's ARG defaults, push, then bump the `FROM` tag. ([#461])
- **Workflow `push` triggers fixed: `main` → `develop` + `master`.** Testing, super-linter, linting, and CodeQL all listed `main` in their push trigger, but the repo uses `develop` → `master` — so push events on the integration branches never fired these workflows. The visible win: Coveralls now receives a base build on `develop` and PR comments stop reporting "No base build found for commit X on develop." CodeQL additionally gains a `pull_request:` trigger so PRs are scanned (previously only the weekly Thursday cron caught anything). ([#462])
- **`misspell` → `codespell` via super-linter.** Drops the standalone reviewdog `misspell` job from `linting.yml` in favor of super-linter's `SPELL_CODESPELL`; unblocks the v2.3.2 release whose misspell check was failing on pre-existing British "kilometres" usages. New `.codespellrc` skip list (chat-command typo aliases like `!commads` / `!quess` are intentional) plus a small ignore list (`caf`, `nd`, `abitrate`). Bundled with ~20 small typo fixes across 14 files (`cant` / `wont` / `doesnt` in comments, plus `delimeter` → `delimiter` with the in-package caller updated). ([#466])
- **Action-version hygiene sweep.** Floats `reviewdog/action-golangci-lint` `v2.0.3` → `v2` (revive job; the errcheck job in the same file already used the floating major) and `super-linter/super-linter` `v8.6.0` → `v8`. Other workflow pins were checked against latest stable and are already current. No-op today; future v2.x / v8.x releases now flow in automatically. ([#436])

### OBS

- **Browser-source on-screens normalized to thirds layout.** Geometry pass on the seven `browser_source` on-screens in `infra/docker/obs/config/Tripbot.json.tmpl`. Left rotator, middle-text, and right rotator each take one 640×47 third of the 1920×1080 canvas in the same y-band as the baked grey overlay boxes (`y=1033..1079`). Leaderboard normalized to 400×400 at (1500, 60). Timewarp banner to 1200×200 at (360, 440). `MIDDLE` group container flattened from a pathological 23×67 internal canvas (item scale 3.72 / group scale 0.488) to a flat 1920×1080, matching `LEFT CORNER` / `RIGHT CORNER`. All affected sources now follow the convention **viewport == on-canvas footprint, scale = 1.0, bounds_type = 0** — removes the implicit "effective size = source × scale" math. Fixes the longstanding bug where middle-text clipped 8 px below the canvas bottom edge. Step 1 of 2; per-onscreen CSS styling (inner `<div>` widths matching the underlying grey-box dimensions, padding, drop-shadows, fade transitions) is queued as a follow-up. ([#467])

### Cleanup

- **Drop unused `TWITCH_AUTH_TOKEN` from env files.** v2.3.0 moved the IRC token from a static env var to a DB-backed OAuth refresh flow; the var is no longer read anywhere in the Go source. Removed from `.env.example`, `.env.development.example`, `.env.testing`, and `infra/docker/env.docker`. Surrounding comments tightened to clarify the new local-vs-cluster split for `TWITCH_CLIENT_ID` / `TWITCH_CLIENT_SECRET` (local dev populates them for `task auth:bootstrap`; cluster pulls from ESO + SM `k8s/tripbot/twitch-creds`). Pairs with [adanalife/infra#438](https://github.com/adanalife/infra/pull/438). ([#465])

## [v2.3.1] — 2026-05-11

Patch release. Tooling-only — bundles the `migrate` CLI and `db/migrate/*.sql` into the runtime image so a cluster k8s Job can run schema migrations without a sibling image. No behavior change at runtime.

### Docker

- **Bundle `migrate/migrate:4` binary + `db/migrate/*.sql` into the tripbot image.** The local-k3d stage-1 cluster has never run schema migrations — the original 2026-05-03 cluster work explicitly noted *"nothing in the cluster is durable yet."* Pre-v2.3.0 tripbot tolerated missing schema (logged at INFO and ran degraded); v2.3.0's `LoadFromDB` boot check makes that an exit-1, so a cluster migration step is now load-bearing. Bundling rather than shipping a sibling `tripbot-migrations` image keeps schema-code version skew impossible by construction (same git SHA → same image), avoids new CI surface, and adds ~10MB binary + 20KB SQL to the runtime image (rounding error). Follow-up infra PR wires a k8s Job using `adanalife/tripbot:v2.3.1`. ([#458])

## [v2.3.0] — 2026-05-10

Minor release. Replaces the static `TWITCH_AUTH_TOKEN` env var (sourced from a third-party token generator) with a self-owned OAuth Authorization Code flow against tripbot's own Twitch dev app. The bot's IRC refresh token now lives in Postgres and rotates hourly via a `pg_try_advisory_lock`-fenced cron job; one-time bootstrap via a new `cmd/auth-bootstrap` CLI. Plus two CI trigger-path filters.

### Authentication

- **`oauth_tokens` table + `pkg/oauthtokens` storage package.** Migration `010_create_oauth_tokens` introduces the table (keyed by `(provider, username)`, stores refresh + access tokens, scopes, expiry, fail counter). The Go-side package wraps it with sqlx queries plus `pg_try_advisory_lock`-backed `TryRefreshLock` so a local-dev tripbot and a cluster pod sharing the same Twitch account can't both rotate the refresh token simultaneously. The lock-id is SHA-256-hashed for a wider key space than `hashtext()`'s 32 bits. ([#452], [#454])
- **`pkg/twitch/authentication.go` rewired off the static env-var token.** `TWITCH_AUTH_TOKEN` env var is no longer required. New `LoadFromDB()` reads the bot's row at boot; missing row → `log.Fatal` with hint pointing at the bootstrap CLI. `IRCAuthToken()` accessor replaces the dropped `AuthToken` global. `RefreshUserAccessToken` uses helix to mint a rotated pair and writes both back to the table; on terminal failure (revoked refresh token) it blanks in-memory state + sends SMS so the bot crashes loudly. Scopes consolidated to a `Scopes` package var (drops `openid`, adds `chat:read` + `chat:edit`). The pre-existing browser-opening block in `chatbot.go` is deleted — `cmd/auth-bootstrap` owns that flow now. ([#455])
- **`/auth/callback` hardened with CSRF state validation + HTML success page.** New `pkg/server/oauthstate` (5-minute TTL, single-use, crypto/rand) generates state at the redirect-initiating side and the callback handler validates it. New `/auth/init` route generates state + 302s to Twitch — provides a cloud-based emergency re-bootstrap path when no laptop is handy. ([#455])
- **New `cmd/auth-bootstrap` CLI + `task auth:bootstrap`.** One-time interactive bootstrap; signs in to Twitch on Dana's laptop, exchanges the code for tokens, derives the username from `helix.GetUsers` (so the row is account-agnostic — bootstrapping the broadcaster account later works identically without an env-var dance), Upserts to the cluster DB via port-forward. After this, all pod restarts and cluster rebuilds are headless. ([#455])
- **`pkg/config` layers `infra/docker/env.docker` after `.env.<env>` for host-side runs.** `docker-compose` does this via `--env-file` inside containers, but host-side binaries (the new bootstrap CLI, host-side cmd/tripbot) previously missed it and failed envconfig for vars that only live in the docker env file (e.g. `TRIPBOT_HTTP_AUTH`). Silent no-op in cluster pods (file not in the image). ([#455])

### CI

- **`obs.yml` PR trigger filtered to OBS-impacting paths.** Skips wasted runs on docs-only / unrelated PRs. The push trigger (develop / `v*` tags) stays unfiltered intentionally — `release.yml` owns the actual release builds and the develop-push smoke test stays useful as a build-soundness check. ([#448])
- **`vlc.yml` push trigger filtered to VLC-impacting paths.** Pairs with [#390](https://github.com/adanalife/tripbot/pull/390)'s PR-side filter; brings develop-push + release-tag pushes in line. ([#447])

## [v2.2.6] — 2026-05-10

Patch release. One small UX addition and one CI hygiene step.

### Chatbot

- **Accept `¡` (U+00A1) as an alternate command prefix.** Spanish-keyboard users can run commands like `¡miles` or `¡location` without switching layouts. A new `normalizeCommandPrefix` helper rewrites a leading `¡` to `!` at the entrance of `runCommand`; the existing `!` path is untouched. Rune-aware (`strings.HasPrefix`/`TrimPrefix`) because `¡` is two bytes in UTF-8 and byte-indexing would mangle it. ([#453])

### CI

- **Super-linter: re-enable `VALIDATE_GITHUB_ACTIONS` (actionlint).** Fixes 4 SC2086 quoting nits in `.github/workflows/vlc.yml` (`$VLC_PORT`, `$GITHUB_ENV`) in a separate prep commit. `VALIDATE_GO_MODULES` was also attempted but reverted — the underlying `golangci-lint` analyzer compiles the module and trips on `vlc/vlc.h: No such file or directory` (same root cause as `VALIDATE_GO` being disabled). The PR body documents the remaining disabled validators with rationale for each, so future re-enables have a starting point. ([#449])

## [v2.2.5] — 2026-05-10

Patch release. One observability gate broaden completing the staging-Sentry pipeline started in v2.2.4, plus a CI improvement and a vlc-server config refactor.

### Observability

- **`pkg/chatbot/log` skips Stackdriver chat logging on staging too.** Both gates (`init()` at `:18` and `ChatMsg()` at `:40`) now early-return on `IsTesting() || IsDevelopment() || IsStaging()`. Pairs with the [adanalife/infra#427](https://github.com/adanalife/infra/pull/427) overlay flip — without this, `ENV=staging` would activate `logging.NewClient` against an empty `GOOGLE_APPLICATION_CREDENTIALS` and `log.Fatalf` at init. Mirrors v2.2.4's launch-plan framing: staging counts for what we explicitly opt in (Sentry), dev-like for everything else. ([#435])

### CI

- **Race detector + coveralls.io coverage publishing.** `testing.yml` now runs `go test -v -race -covermode=atomic -coverprofile=coverage.out ./...` and publishes via `jandelgado/gcov2lcov-action` + `coverallsapp/github-action`. Salvaged from closed PR [#126](https://github.com/adanalife/tripbot/pull/126); pairs with the in-progress unit-testing improvements tracked in `vault/tripbot/TODO.md`. ([#438])

### Cleanup

- **vlc-server tuning flags now optional env vars.** `VLC_FILE_CACHING`, `VLC_AVCODEC_HW`, `VLC_VOUT`, `VLC_CANVAS_WIDTH`, `VLC_CANVAS_HEIGHT` move from hardcoded values to env-var overrides; all default to today's values, so this is a pure refactor. Resolves the `//TODO: break some of these into ENV vars` comment in `pkg/vlc-server/vlc.go`. ([#445])

## [v2.2.4] — 2026-05-09

Patch release. Sentry SDK gets a long-overdue bump and the error-reporting gate broadens to fire from staging too — pairs with infra-side ESO wiring that delivers per-app DSNs into stage-1. Plus one Dockerfile cleanup.

### Observability

- **Sentry reports from staging too.** `pkg/errors` was gated to `IsProduction()` only, so the launch-plan staging soak would have silently lost exceptions. Broadened to `IsProduction() || IsStaging()`; `IsStaging()` is added to the `Config` interface (both `TripbotConfig` and `VlcServerConfig` already implement it). ([#433])
- **`getsentry/sentry-go` bumped 0.11.0 → 0.46.2.** Five years of upstream. The negroni middleware was split into its own submodule (`getsentry/sentry-go/negroni`), pulling in `urfave/negroni/v3` as indirect; existing call sites — `Init`, `AddBreadcrumb`, `CaptureException`, `Flush`, `sentrynegroni.New` — compile unchanged. Supersedes Dependabot bump #361 (which only went to v0.29.1). ([#433])

### Cleanup

- **`infra/docker/vlc/Dockerfile` Go bump 1.21.13 → 1.26.3.** Last stale `1.21` reference in the repo; the vlc image curl-installs Go separately because its Ubuntu 24.04 base needs system `libvlc-dev` and can't ride the `golang:1.26-bookworm` image like tripbot/test do. ([#432])

## [v2.2.3] — 2026-05-09

Patch release. Four observability follow-ups to v2.2.0's OpenTelemetry wiring, plus one Dependabot bump.

### Observability

- **Go runtime metrics flow through OTLP.** `runtime.Start()` from `go.opentelemetry.io/contrib/instrumentation/runtime` is now wired into `pkg/telemetry/Init` so `process.runtime.go.{goroutines,gc.*,mem.heap.*}` reach Grafana Cloud. Previously the runtime collectors lived in Prometheus's default registry — scraped via `/metrics` but never sent over OTLP — so the Go-runtime dashboard shipped on the infra side was a placeholder. ([#427])
- **`http.route` populated on metrics and traces.** Every `pkg/server` and `pkg/vlc-server` route now sets `http.route` via `otelhttp.Labeler` and the active span, using mux's `{var}` syntax to keep cardinality bounded. Negroni doesn't surface the underlying mux template to `otelhttp.NewHandler`, so the label was empty before this; the HTTP Routes Explorer dashboard can now group by `http_route` for proper per-endpoint breakdowns. ([#428])
- **Span per inbound chat message + per cron tick.** `pkg/chatbot/handlers.go`'s `PrivateMessage` opens a `chatbot.handle_message` span around login + dispatch, with `twitch.user` set on entry and `twitch.command` set inside `runCommand` only when the message is `!`-prefixed (cardinality control). `cmd/tripbot/tripbot.go`'s `scheduleBackgroundJobs` wraps each callback in a `tracedJob` helper that opens a `cron.<name>` span. The Twitch IRC path was completely invisible to tracing before this. ([#431])
- **Drop dangling `vlc_server_http_duration` histogram.** `pkg/instrumentation/common.go` declared an OTel histogram that was never `Record`ed; the live HTTP duration metric comes from the `slok/go-http-metrics` Negroni middleware in `pkg/server`. Removing the dead declaration leaves nothing of substance in `common.go`, so the file goes too. ([#429])

### CI

- **`github/codeql-action` bumped 3 → 4.** Dependabot. ([#414])

## [v2.2.2] — 2026-05-08

Stage-1 verification of v2.2.1 surfaced two cosmetic-but-correctness-bearing follow-ups; this release picks them up.

### Observability

- **`vlc-server`'s `/version` now populates `sha` and `built_at`.** The vlc Dockerfile was building from a single-file path (`cmd/vlc-server/vlc-server.go`), which bypasses Go's automatic `-buildvcs` VCS metadata embedding. Switched to the package path (`./cmd/vlc-server`) — same form `tripbot` was already using. `/etc/tripbot/sha` is unaffected (that's plumbed via the `SHA` build-arg). ([#423])
- **`/version` no longer returns a `dirty` field.** `runtime/debug.ReadBuildInfo()`'s `vcs.modified` read `true` even on a build of the clean tagged v2.2.1 commit (likely an `actions/checkout@v6` LFS-materialization artifact). Until the root cause is understood, a perpetually-true `dirty` field is misleading; remove from the JSON shape on both Go services. Restoring is tracked as a follow-up. ([#423])

### CI

- **`ankitvgupta/pr-updater` bumped 1.4.0 → 1.4.1.** Dependabot. ([#415])

## [v2.2.1] — 2026-05-08

Re-ship of v2.2.0 with corrected version stamping. v2.2.0's `release.yml` run failed at the new `Verify version stamping` gate on every per-arch build leg, so the multi-arch `:2.2.0` and `:latest` manifests were never assembled — only broken per-arch tags reached Docker Hub. v2.2.1 publishes those manifests correctly.

### CI

- **Use bare semver for the `VERSION` build-arg and the verify step's image name.** `docker/metadata-action`'s `outputs.version` reflects the per-arch `flavor: suffix=-${arch}`, so it carried the arch suffix already. Plumbing it as the `VERSION` build-arg stamped binaries with `service.version=2.2.0-amd64` (defeating the v2.2.0 release's clean version-stamping); the verify step then double-applied the arch suffix and tried to pull `:2.2.0-amd64-amd64`, which doesn't exist. New `Resolve bare version` step per build job exposes `${GITHUB_REF_NAME#v}` (e.g., `2.2.0`) for both consumers; the matrix arch only goes onto the published per-arch tag. ([#421])

## [v2.2.0] — 2026-05-08

Ships first-class build-version exposure across all three containers (HTTP `/version`, `/etc/tripbot/{version,sha}`, startup log lines) and the Go-side of OpenTelemetry instrumentation — `service.version` now reads as the real release tag in Grafana instead of `dev`. Pairs with the infra-side Grafana Cloud OTLP wiring landing separately. Plus a Go toolchain bump.

### Observability

- **OpenTelemetry tracing, metrics, and logging.** New `pkg/telemetry` brings up OTel SDK providers from `OTEL_*` env vars, no-ops cleanly when `OTEL_SDK_DISABLED=true` or no endpoint is set. Both `tripbot` and `vlc-server` mains pass their `version` string into `telemetry.Init(...)` for `service.version` resource attribution; the HTTP servers wrap their handlers with `otelhttp.NewHandler` for trace propagation. Grafana Cloud OTLP creds are injected via the `grafana-cloud-otlp` Secret on stage-1 (see infra side). ([#411])
- **Build version surfaces on every container.** Three new ways to read what's deployed: HTTP `GET/HEAD /version` on the Go services returning JSON `{tag, sha, built_at, dirty}`; `/etc/tripbot/version` + `/etc/tripbot/sha` baked into all three images at build time; container startup logs include the version on the first line. The Go `tag` ldflag and `runtime/debug.ReadBuildInfo()` populate the JSON together. ([#419])

### CI

- **`release.yml` gates on version stamping.** New `.github/scripts/verify-stamped-image.sh` runs after each per-arch build/push, pulls the image, and asserts `/etc/tripbot/{version,sha}` match the release tag and `github.sha`. Fails the workflow if any image reads `version=dev` so a regression in the build-args plumbing can't ship a tagged release with placeholder values. ([#419])
- **PR-time CI verifies version files.** `tripbot.yml` / `vlc.yml` / `obs.yml` each `docker exec test -s /etc/tripbot/{version,sha}` after bring-up; the Go containers also curl `/version` to confirm the endpoint serves. Catches Dockerfile-level regressions at PR time. ([#419])

### Tooling

- **Go 1.26.3.** Bumps the Go toolchain pin (test-image base + `go.mod`) to keep us on a current release. ([#417])

## [v2.1.0] — 2026-05-07

Closes a `/auth/twitch` token-leak (wrong non-empty secrets falling through to a 200 with the JSON tokens), adds stage-1 release Taskfile targets so deploy + smoke happen via one command, surfaces OBS crash-dialog state to k8s healthchecks, and drops vestigial onscreens disk-write code. Plus a CI hygiene sweep.

### Security

- **`/auth/twitch` no longer 200s on a wrong non-empty secret.** `isValidSecret` was misnamed and had inverted semantics — for any non-empty wrong `auth=`, the guard fell through and the endpoint returned the JSON-encoded Twitch tokens. Empty/missing `auth=` was correctly 404'd, masking the bug. Renamed to `isInvalidSecret`, dropped the `!` at the call site, flipped the test that pinned the bug. ([#391])

### Cleanup

- **Onscreens disk-write code removed.** With the post-#373 vlc/obs split, OBS renders via browser sources against `vlc-server`'s HTTP endpoints and no longer reads files out of `/opt/data/run/`. Drops the `Snapshot` type, the disk-write paths in `onscreens-server`, the embedded flag-placeholder write, and the unread `*_DIR` env vars from the docker-compose. Lets `infra` drop the `onscreens` PVC + `podAffinity` blocks (separate PR). ([#409])

### OBS

- **Crash-dialog state surfaces in the healthcheck.** When OBS hits the safe-mode crash dialog (e.g. after an unclean shutdown), the process is technically up but the canvas is frozen on a modal. The healthcheck now detects the dialog and reports unhealthy so k8s can restart the pod. ([#380])

### Release

- **`task release:smoke:stage-1`** — combined Taskfile target that applies `k8s/overlays/stage-1`, waits for the four rollouts (tripbot, vlc-server, obs, cloudflared), then hits both local-cluster and `tripbot.whalecore.com` health endpoints. Plus split-out `release:smoke:whalecore` and `release:smoke:local` for re-running just the public or in-cluster checks. Used by Phase 4 of the release checklist. ([#402])

### CI

- **`actions/checkout` v4 → v6** across remaining workflows. ([#404])
- **`linting.yml` rewritten for `golangci-lint` v2.** v1 was EOL'd upstream; v2 changed config schema (`linters.enable` → `linters.default`), output format flags, and `--timeout` semantics. ([#398])
- **`vlc.yml` PR runs filtered to VLC-impacting paths.** Avoids spending CI minutes rebuilding the VLC image on PRs that only touch `pkg/server/`, the OBS container, or docs. ([#390])

## [v2.0.2] — 2026-05-06

Maintenance release. Dead-code cleanup, OBS profile + scene polish, and a sweep through stale GitHub Actions versions. No behavior changes for the bot or the stream.

### Cleanup

- **Dead `DISABLE_OBS` plumbing removed.** The env var was masking pre-#373 supervisord/OBS-PID code in the VLC container that no longer made sense after the vlc/obs split. Drops the `[program:obs]` block from `script/container_startup.sh`, the OBS-PID branch from vlc-server's `/health`, the `DISABLE_OBS` pass-throughs in compose, the orphaned `script/x11/start-obs.sh`, and the pre-revival `Dashcam_Scenes.docker.json` (already ported in #371). ([#388])

### OBS

- **Profile renamed `Untitled` → `ADanaLife`** in the seeded profile dir + `--profile` flag, so the OBS profile dropdown shows the brand instead of the placeholder. ([#389])
- **`feh` installed** so fluxbox's `fbsetbg` stops logging "I can't find an app to set the wallpaper with..." every boot. OBS owns the framebuffer; this is pure log-noise hygiene. ([#393])
- **Test scene background** switched from a muddy brown to Twitch chat dark (`#18181B`). Only affects the Test fallback scene — Main hides the background behind dashcam + overlays. ([#394])

### CI

- **`actions/checkout` v2 → v4** across `codeql-analysis`, `linting`, `super-linter`, `update-pr` (v2 was on Node 12 EOL). ([#395])
- **`github/codeql-action` v1 → v3** in `codeql-analysis` (v1 was Node 12 EOL). ([#396])
- **Trivially-bumpable action versions** across `.github/workflows/`: `actions/cache` v4 → v5, `docker/build-push-action` v5 → v7, `docker/login-action` v3 → v4, `docker/metadata-action` v5 → v6, `docker/setup-buildx-action` v3 → v4, `dorny/paths-filter` v3 → v4, `Ilshidur/action-discord` 0.3.2 → 0.4.0, `anothrNick/github-tag-action` 1.71.0 → `1` (floating major). All Node-runtime upgrades with no input/output API changes for our usage. ([#397])
- **`super-linter` v4.5.1 → v8.6.0** — four-major jump pinned explicitly to v8.6.0. Org rename `github/super-linter` → `super-linter/super-linter`; `VALIDATE_KUBERNETES_KUBEVAL` → `VALIDATE_KUBERNETES_KUBECONFORM`; removed `VALIDATE_SQL` (sql-lint deleted upstream). v6+ diff-mode requires `fetch-depth: 0` on the checkout step. v5+-newly-enabled linters disabled to keep v4 behavior parity (re-audit tracked separately). ([#399])

## [v2.0.1] — 2026-05-05

### Added

- **Multi-arch tripbot and vlc images.** v2.0.0 only published amd64 manifests for these two; v2.0.1 ships native arm64 builds alongside, completing the stage-1 arm64 deploy story. ([#385])

## [v2.0.0] — 2026-05-05

The "back online" release. After roughly four years of dormancy, the entire container stack has been rebuilt on a current base, the OBS container has been revived with from-source CEF support on arm64, and the release pipeline is functional again. This is the first release on which the full local k3d cluster comes up healthy.

### Container stack rebuild

- **Base bumped to Ubuntu 24.04** across tripbot, vlc, and obs images. ([#369], [#372])
- **Go 1.21**, replacing Go 1.17 in the tripbot builder. ([#372])
- **Postgres 16** for the dev/test database, up from Postgres 11. ([#372])
- **Compose V2** for all local dev workflows; deprecated `docker-compose` syntax removed. ([#367])
- **GitHub Actions versions modernized** — `actions/checkout@v4`, `docker/build-push-action@v5`, `docker/setup-buildx-action@v3`, etc. The legacy `bump-and-release.yml` was retired in favor of a new `auto-tag.yml` (push to master → semver tag). ([#367], [#383])

### OBS container revival

- **Revived OBS container** from a minimal Ubuntu 24.04 base with a working CEF browser. ([#368])
- **OBS scenes + overlays ported** from the pre-rewrite collection into `infra/docker/obs/config/Tripbot.json.tmpl`, with dummy onscreen fixtures so the scene renders in dev without the bot writing state. ([#371])
- **Multi-arch OBS image.** amd64 uses the OBS PPA (`ppa:obsproject/obs-studio`); arm64 builds OBS from source against the official aarch64 CEF tarball — the PPA has no arm64 channel and Ubuntu universe ships obs-studio with CEF stripped. Both variants ship `obs-browser.so` and load browser sources cleanly. ([#377])
- **CEF launch defects fixed:** `chmod 4755 chrome-sandbox` in the runtime stage so render processes can launch under their own namespace; `BrowserHWAccel=false` seeded in `user.ini` so CEF avoids the failing arm64 swiftshader-webgl path. ([#377])
- **OBS window centered** in the Xvfb display via fluxbox `apps` rule. ([#378])

### VLC server

- **VLC container introduced** to serve the dashcam stream (Go-based `vlc-server` binary plus apt VLC + RTSP plugin). ([#366])
- **VLC → OBS over RTSP.** OBS consumes `rtsp://vlc-server:8554/dashcam` via an `ffmpeg_source`, replacing the old window-capture-of-VLC approach. ([#370])

### Onscreens architecture

- **Browser-source onscreens.** vlc-server serves a per-onscreen HTML page and a `state.json` polling endpoint; each onscreen renders as an OBS `browser_source`, so content updates flow over HTTP instead of through shared container state. ([#379])
- **`/onscreens/{name}/{show,hide}` HTTP API** on vlc-server for the bot to drive content updates.

### CI and release pipeline

- **Workflows split per container** — `tripbot.yml`, `obs.yml`, `vlc.yml`, plus a tag-only `release.yml`. Eliminates the OBS-amd64 build duplication that had been running in two places. ([#382])
- **Multi-arch release pipeline.** OBS publishes per-arch `:<version>-amd64` / `:<version>-arm64` tags plus a multi-arch manifest list at `:<version>` that auto-resolves on the deploy node's architecture. ([#383])
- **CI build-time speedups.** `dorny/paths-filter` skips the slow arm64 OBS leg on PRs that don't touch OBS image inputs (saves ~30 min CEF compile per PR); buildx + GHA layer caching for VLC and tripbot main image. ([#381], [#382])
- **Auto-tag on master push.** Pushes to master fire `auto-tag.yml`, which derives the next semver tag from commit-message keywords (`#major`/`#minor`/`#patch`, default patch) and pushes it via PAT so the resulting tag fires `release.yml` automatically. ([#383])
- **OBS container-modal healthcheck.** OBS container reports unhealthy when the OBS-32 safe-mode crash dialog is up — detected by walking `_NET_CLIENT_LIST` via `xprop` for a window matching `WM_CLASS=obs` + `_NET_WM_STATE_MODAL` + `WM_NAME ~ Crash Detected`.

### Testing

- **Foundational unit-test suite + dockerized Taskfile runner.** `task test` brings up the full container stack and runs Go tests inside the tripbot image, matching the runtime environment. ([#376])

### Removed

- **`docker.yml`** retired in favor of the per-container split. ([#382])
- **`bump-and-release.yml`** retired — references removed `cmd/tripbot/.goreleaser.yml`, deleted Sentry projects, and triggered on `main` (not the default branch). Replaced by `auto-tag.yml`. ([#383])

### Notes

- Last shipped release before this was **v1.8.0 (2022-01-02)**. Tags `v1.9.0` and `v1.9.1` were published by the now-retired auto-bump workflow during dependabot churn but never represented a coherent release; treat v2.0.0 as the successor to v1.8.0.
- Local dev still uses `bin/devenv` to wrap docker compose; the per-container CI workflows mirror that wrapper's overlay layering.

## Earlier history (pre-revival)

The repo dates to 2018. v1.x covered the original development and steady-state operation of the Twitch slow-tv stream. Highlights, summarized:

- **2018–2019** — Initial Twitch chat bot, IRC integration, video selection and dashcam playback, leaderboards, scoreboards.
- **2020–2021** — Heavy feature work: OAuth flows, bonus miles / followers / subs, OCR-driven location detection, audio engine, real-time scene control. Most of the v1.x minor releases (v1.0–v1.7) covered this period.
- **2022** — Removal of OCR ([#79]) and the MPD audio engine ([#78]) as those features were retired. **v1.8.0 (2022-01-02)** was the last real release.
- **2022–2026** — Dormant. Dependabot kept Go modules, action versions, and the Go base image bumped, and the now-retired auto-bump workflow tagged `v1.9.0` and `v1.9.1` along the way without a substantive feature delta.
- **2026** — Full revival starting with [#366]. See v2.0.0 above.

[v2.1.0]: https://github.com/adanalife/tripbot/releases/tag/v2.1.0
[v2.0.2]: https://github.com/adanalife/tripbot/releases/tag/v2.0.2
[v2.0.1]: https://github.com/adanalife/tripbot/releases/tag/v2.0.1
[v2.0.0]: https://github.com/adanalife/tripbot/releases/tag/v2.0.0

[#78]: https://github.com/adanalife/tripbot/pull/78
[#79]: https://github.com/adanalife/tripbot/pull/79
[#366]: https://github.com/adanalife/tripbot/pull/366
[#367]: https://github.com/adanalife/tripbot/pull/367
[#368]: https://github.com/adanalife/tripbot/pull/368
[#369]: https://github.com/adanalife/tripbot/pull/369
[#370]: https://github.com/adanalife/tripbot/pull/370
[#371]: https://github.com/adanalife/tripbot/pull/371
[#372]: https://github.com/adanalife/tripbot/pull/372
[#376]: https://github.com/adanalife/tripbot/pull/376
[#377]: https://github.com/adanalife/tripbot/pull/377
[#378]: https://github.com/adanalife/tripbot/pull/378
[#379]: https://github.com/adanalife/tripbot/pull/379
[#381]: https://github.com/adanalife/tripbot/pull/381
[#382]: https://github.com/adanalife/tripbot/pull/382
[#383]: https://github.com/adanalife/tripbot/pull/383
[#385]: https://github.com/adanalife/tripbot/pull/385
[#388]: https://github.com/adanalife/tripbot/pull/388
[#389]: https://github.com/adanalife/tripbot/pull/389
[#393]: https://github.com/adanalife/tripbot/pull/393
[#394]: https://github.com/adanalife/tripbot/pull/394
[#395]: https://github.com/adanalife/tripbot/pull/395
[#396]: https://github.com/adanalife/tripbot/pull/396
[#397]: https://github.com/adanalife/tripbot/pull/397
[#399]: https://github.com/adanalife/tripbot/pull/399
[#380]: https://github.com/adanalife/tripbot/pull/380
[#390]: https://github.com/adanalife/tripbot/pull/390
[#391]: https://github.com/adanalife/tripbot/pull/391
[#398]: https://github.com/adanalife/tripbot/pull/398
[#402]: https://github.com/adanalife/tripbot/pull/402
[#404]: https://github.com/adanalife/tripbot/pull/404
[#409]: https://github.com/adanalife/tripbot/pull/409
[#411]: https://github.com/adanalife/tripbot/pull/411
[#417]: https://github.com/adanalife/tripbot/pull/417
[#419]: https://github.com/adanalife/tripbot/pull/419
[#421]: https://github.com/adanalife/tripbot/pull/421
[#415]: https://github.com/adanalife/tripbot/pull/415
[#423]: https://github.com/adanalife/tripbot/pull/423
[#414]: https://github.com/adanalife/tripbot/pull/414
[#427]: https://github.com/adanalife/tripbot/pull/427
[#428]: https://github.com/adanalife/tripbot/pull/428
[#429]: https://github.com/adanalife/tripbot/pull/429
[#431]: https://github.com/adanalife/tripbot/pull/431
[#432]: https://github.com/adanalife/tripbot/pull/432
[#433]: https://github.com/adanalife/tripbot/pull/433
[#435]: https://github.com/adanalife/tripbot/pull/435
[#438]: https://github.com/adanalife/tripbot/pull/438
[#445]: https://github.com/adanalife/tripbot/pull/445
[#449]: https://github.com/adanalife/tripbot/pull/449
[#453]: https://github.com/adanalife/tripbot/pull/453
[#447]: https://github.com/adanalife/tripbot/pull/447
[#448]: https://github.com/adanalife/tripbot/pull/448
[#452]: https://github.com/adanalife/tripbot/pull/452
[#454]: https://github.com/adanalife/tripbot/pull/454
[#455]: https://github.com/adanalife/tripbot/pull/455
[#458]: https://github.com/adanalife/tripbot/pull/458
[#461]: https://github.com/adanalife/tripbot/pull/461
[#462]: https://github.com/adanalife/tripbot/pull/462
[#436]: https://github.com/adanalife/tripbot/pull/436
[#465]: https://github.com/adanalife/tripbot/pull/465
[#466]: https://github.com/adanalife/tripbot/pull/466
[#467]: https://github.com/adanalife/tripbot/pull/467
