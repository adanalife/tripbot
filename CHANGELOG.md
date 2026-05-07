# Changelog

All notable changes to TripBot. Format follows [Keep a Changelog](https://keepachangelog.com); versioning follows [Semantic Versioning](https://semver.org).

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
