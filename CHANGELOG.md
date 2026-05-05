# Changelog

All notable changes to TripBot. Format follows [Keep a Changelog](https://keepachangelog.com); versioning follows [Semantic Versioning](https://semver.org).

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
- **OBS-32 first-run handling** — defensive `rm -f global.ini` before copying the seeded `user.ini`, plus `FirstRun=true` in the seed to skip the auto-config wizard.
- **OBS window centered** in the Xvfb display via fluxbox `apps` rule. ([#378])

### Dashcam pipeline

- **VLC container introduced** to serve the dashcam stream (Go-based `vlc-server` binary plus apt VLC + RTSP plugin). ([#366])
- **VLC → OBS over RTSP.** OBS consumes `rtsp://vlc-server:8554/dashcam` via an `ffmpeg_source`, replacing the old window-capture-of-VLC approach. ([#370])
- **Shared `onscreens` volume** between vlc and obs, with the compose wiring fixed so onscreen state files reach both containers. ([#373])

### Onscreens architecture

- **Browser-source onscreens** replace the old docker-compose named-volume HACK between vlc-server and OBS. vlc-server now serves a per-onscreen HTML page and a `state.json` polling endpoint; each onscreen is an OBS `browser_source`. ([#379])
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
[#373]: https://github.com/adanalife/tripbot/pull/373
[#376]: https://github.com/adanalife/tripbot/pull/376
[#377]: https://github.com/adanalife/tripbot/pull/377
[#378]: https://github.com/adanalife/tripbot/pull/378
[#379]: https://github.com/adanalife/tripbot/pull/379
[#381]: https://github.com/adanalife/tripbot/pull/381
[#382]: https://github.com/adanalife/tripbot/pull/382
[#383]: https://github.com/adanalife/tripbot/pull/383
[#385]: https://github.com/adanalife/tripbot/pull/385
