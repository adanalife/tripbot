# Changelog

<!-- markdownlint-disable MD024 -->
<!-- Duplicate sibling headings (Cleanup, OBS, CI, etc.) are intentional — same section names recur per release entry. Keep-a-Changelog convention. -->

All notable changes to TripBot. Format follows [Keep a Changelog](https://keepachangelog.com); versioning follows [Semantic Versioning](https://semver.org).

Unreleased changes live as fragment files in [`changelog.d/`](changelog.d/) and are assembled here at release time by [towncrier](https://towncrier.readthedocs.io). See the [Changelog section of the README](README.md#changelog).

<!-- towncrier release notes start -->

## [v3.10.1] — 2026-07-03

### Chatbot

- Rollup schema: `user_rollups` + `rollup_watermarks` + `scoreboard_snapshots` tables — derived-state substrate for the events rollup reconciler (worker in a follow-up PR) ([#1007](https://github.com/adanalife/tripbot/pull/1007))
- Events rollup reconciler: `user_rollups` aggregates recomputed from the events table on an id watermark, plus once-only month-end scoreboard snapshots — derived state for `!leaderboard` async, `!lastmonth`, and cross-platform stats ([#1009](https://github.com/adanalife/tripbot/pull/1009))

### Onscreens

- Fixed the occasionally-blank rotator corner: a near-invisible heartbeat pixel on every onscreen page forces CEF to deliver a continuous frame stream to OBS, so a dropped frame after the hourly browser-source refresh self-heals in ~500ms instead of staying blank until the next rotation (up to 45s — indefinitely for middle-text). ([#1006](https://github.com/adanalife/tripbot/pull/1006))

### Deploy / Infra

- Identity/DB/observability Secrets now sync from SSM Parameter Store (the free `aws-parameterstore` ESO store) instead of AWS Secrets Manager.

### Cleanup

- Remove dead code flagged by a ponytail over-engineering audit: unused helpers (`ReadPidFile`, `PidExists`, `InvertMap`, `Base64Encode`/`Base64Decode` and the now-obsolete base64-in-URL regression test — overlay content moved to NATS), unused config (`IsTesting` on all three config types, the `GoogleMapsStyle` and `TimestampsToTry` vars), three caller-less `pkg/twitch` free-function shims, and the never-set `TripbotConfig.OutputChannel` field plus the dead branch that read it.
- Replace a handful of hand-rolled loops/comparisons with their stdlib equivalents (`slices.Contains`, `slices.Sort`, `strings.EqualFold`). No behavior change.

### Misc

- Mark the chatbot/eventbus NATS publisher interfaces as ponytail debt (they duplicate `natsclient.Publisher`); no functional change.

## [v3.10.0] — 2026-07-01

### Onscreens

- Platform-scope the onscreens overlay NATS subjects (trailing `.<platform>` leaf) so each per-platform `onscreens-server` only renders its own stream's overlays. Fixes the Twitch rotating leaderboard (and `!flag`/`!state`/middle-text) leaking onto the bot-less YouTube stream — leaderboards stay off YouTube until inbound chat, and therefore miles/guesses, returns there. ([#990](https://github.com/adanalife/tripbot/pull/990)) ([#990](https://github.com/adanalife/tripbot/pull/990))
- Refresh the bot-less YouTube promo copy: the two on-screen rotator corners now advertise different actions (left → chat live on Twitch, right → subscribe on YouTube + journey flavor) instead of echoing the same "follow on Twitch" line, the `!help` rotator says "subscribe" rather than Twitch's "follow", and both pools gain variety plus a guess/miles/leaderboard tease that points viewers to where those features work. ([#991](https://github.com/adanalife/tripbot/pull/991)) ([#991](https://github.com/adanalife/tripbot/pull/991))
- The two overlay corners no longer advertise the same command at the same time — when one rotator shows e.g. `!location`, the other skips any line mentioning that command (relaxing only if the exclusion would leave it blank). Internally the left/right rotators are now one parameterized `rotator` type instead of two near-identical copies.

### Fixes

- Keep audible music on the Twitch stream when SomaFM drops. A new OBS background-audio watchdog watches the `Groove Salad Classic` (SomaFM) source and, when it goes silent — the EOF-wedge where OBS stops retrying, single-edge jitter, or a full SomaFM edge outage, all seen on 2026-06-23 with no self-heal — swaps the source onto the local license-clean Car Hum bed so the stream isn't silent, then swaps it back once a SomaFM edge is serving bytes again. Adds `obs_background_audio_level_db`, `obs_background_audio_playing`, `obs_background_audio_on_fallback`, and `somafm_reachable` metrics to alert on. ([#993](https://github.com/adanalife/tripbot/pull/993))
- Fix the SomaFM audio-fallback swap-back: the reachability probe sent Go's default `User-Agent`, which SomaFM's ICEcast edges reject (connection closed before any response), so the probe always reported unreachable and the stream could never return from the local Car Hum bed to SomaFM. The probe now sends a real User-Agent.
- Fix two issues in the SomaFM audio-fallback watchdog found during stage testing: the fallback now loops the local Car Hum bed (it was playing once and then going silent), and the SomaFM reachability probe now logs why it fails and uses a fresh connection per check (a stale pooled connection could otherwise strand the stream on the fallback bed).

### Cleanup

- Drop the `go-spew` dependency — its only live use was a single debug dump, now done with `fmt.Printf`.

## [v3.9.4] — 2026-06-23

### VLC

- Stamp a `service_platform` label (twitch/youtube) on every vlc-server and OBS metric series, so the per-platform encoder instances stay distinct instead of colliding on identical Prometheus series identities. ([#983](https://github.com/adanalife/tripbot/pull/983))

## [v3.9.3] — 2026-06-22

### Platform gateway

- Route prod tripbot-youtube outbound chat sends through gateway-youtube (gateway owns the YouTube token). ([#971](https://github.com/adanalife/tripbot/pull/971))

### Console / API

- Publish the current YouTube broadcast (videoId + privacy) on `tripbot.<env>.youtube.broadcast` so the console can link to and embed the live broadcast directly — needed for an unlisted broadcast, whose channel/handle `/live` redirect only resolves a public stream. A YouTube-instance discovery cron polls the gateway's new `/v1/broadcast` endpoint every 2 minutes (running regardless of `YOUTUBE_INBOUND_ENABLED`, so it works on the bot-less prod instance).

### Cleanup

- Finished the OBS decommission: removed the `bin/obs-browser-refresh` / `bin/obs-media-restart` operator scripts and the `obs:browser:refresh` task (they live in the [adanalife/obs](https://github.com/adanalife/obs) repo now), and repointed `pkg/chatbot/carsound.go`'s carhum/scene-config comments at the obs repo.
- Removed OBS from this repo. The OBS container image, its build/release workflows, and its cdk8s deployment now live in the standalone [adanalife/obs](https://github.com/adanalife/obs) repo; tripbot keeps only the runtime OBS WebSocket controller (`pkg/obs`) that drives the remote OBS instance.

## [v3.9.2] — 2026-06-22

### Chatbot

- Bot-less YouTube mode: `YOUTUBE_INBOUND_ENABLED` (default true) gates the inbound chat poll. When off, the Chatter/!help rotator serves promotional copy pointing viewers at the live Twitch channel instead of commands that can't respond. ([#962](https://github.com/adanalife/tripbot/pull/962))

### Onscreens

- On-screen left/right rotators serve promotional copy (watch & chat live on Twitch, interactivity coming soon) on a bot-less YouTube instance instead of advertising chat commands that no-op there. ([#962](https://github.com/adanalife/tripbot/pull/962))
- On a bot-less YouTube stream the left/right rotators now surface the currently-playing clip's location (`📍 City, State`) and date (`📅 Date`) in place of the command hints — the info the `!location` / `!date` / `!state` commands would return, shown passively since no command can respond. tripbot pushes the data on a timer (city geocoded ≤1×/5min and cached); it mixes with the Twitch-CTA promo lines and falls back to promo-only when no fresh data. ([#966](https://github.com/adanalife/tripbot/pull/966))

### Deploy / Infra

- Prod YouTube launches bot-less while the YouTube Data API quota extension is pending — the inbound chat poll is off (it would blow the default quota at prod's 2s floor), so the bot streams and posts but answers no commands. Stage keeps the full bot for testing. ([#962](https://github.com/adanalife/tripbot/pull/962))
- Unpark the prod-1 YouTube app stack — `tripbot`/`vlc`/`onscreens`-youtube go live (unlisted, bot-less) to burn in before the public launch. ([#968](https://github.com/adanalife/tripbot/pull/968))

## [v3.9.1] — 2026-06-21

### CI / Tooling

- Exempt the standing back-merge PR (`backmerge/master-to-develop`) from the conventional PR-title check — it merges with a merge commit, so its title never becomes a commit subject. ([#952](https://github.com/adanalife/tripbot/pull/952))

## [v3.9.0] — 2026-06-21

Minor release. Completes the YouTube→platform-gateway migration: with both chat directions routed through `gateway-youtube`, tripbot holds no YouTube token at any point in its lifecycle, and the in-process YouTube auth + Data-API client are deleted. Prod now serves the dashcam corpus from the minipc's local NVMe instead of the NAS (with an instant NFS fallback), onscreens-server reports to its own Sentry project, and a consumer-side `tripbot_gateway_up` reachability metric lands. Rounds out with a batch of release-engineering upgrades — towncrier changelog fragments, Conventional Commits enforcement on commits and PR titles, and arm64 release builds routed to a self-hosted rpi5 runner with GitHub-hosted fallback.

### Platform gateway

- **YouTube outbound chat-send routes through `gateway-youtube` unconditionally — the `chatbot.youtube_gateway` flag is gone.** A `tripbot-youtube` instance wired with `YOUTUBE_API_URL` now sends through the gateway with no runtime toggle. Unlike the Twitch cutover (which keeps a flag to de-risk the live-prod swap), YouTube has no live-prod stakes, so it cuts straight over — a revert is a `git revert` + redeploy. Drops the `flaggedYouTubeSend` wrapper; migration 024 removes the seeded flag row. The inbound chat poll still stays in-process (no gateway streaming endpoint yet). ([#935](https://github.com/adanalife/tripbot/pull/935))
- **`tripbot-youtube` reads inbound chat from the gateway — no YouTube token in tripbot at runtime.** When wired with `YOUTUBE_API_URL`, the youtube instance now drives `gateway-youtube`'s `GET /v1/chat/inbound` poll loop instead of calling `liveChatMessages.list` in-process. Combined with the outbound send ([#935](https://github.com/adanalife/tripbot/pull/935)), both chat directions flow through the gateway, so `tripbot-youtube` no longer loads the `oauth_tokens` row — the gateway holds the YouTube credential. The in-process poll + token load remain the fallback for an un-wired instance (the OAuth bootstrap stays in tripbot until a later phase). Requires the matching `gateway-youtube` release ([platform-gateway#20](https://github.com/adanalife/platform-gateway/pull/20/changes)) live first. ([#936](https://github.com/adanalife/tripbot/pull/936))
- **`tripbot_gateway_up` reachability metric.** Tripbot now records a gauge on every platform-gateway call — 1 when the call gets an HTTP response (gateway reachable), 0 on a transport failure — at the two `http.Do` chokepoints in the gateway client, covering all v1 calls and the chat-send path. It's the consumer-side signal for a gateway down-alert: it catches "the bot can't reach the gateway" (network partition, gateway crash, DNS), which the gateway's own `platform_gateway_up` process-liveness gauge can't report. ([#942](https://github.com/adanalife/tripbot/pull/942))

### Onscreens

- **onscreens-server reports to its own Sentry project.** Its Sentry `envFrom` swaps from the shared `sentry-vlc-server` Secret to a dedicated `sentry-onscreens-server`, so onscreens errors no longer masquerade as vlc-server's. SM container + ExternalSecret provisioned in infra; per the one-project-per-binary model. ([#944](https://github.com/adanalife/tripbot/pull/944))

### VLC

- **`dashcam_source` flag flips vlc between the NFS corpus and a node-local cache.** A new env knob (`nfs` | `local`, default `nfs`) selects which PVC vlc mounts the dashcam corpus from — the NFS-backed `vlc-dashcam` or the node-local `vlc-dashcam-local` cache on the minipc's NVMe. Flipping back to `nfs` is the instant fallback while the local copy is (re)populated. The local PVC + its NFS→local copy Job are provisioned by infra's `dashcam_local_enabled` flag ([infra#772](https://github.com/adanalife/infra/pull/772/changes)); this side only picks the claim. Default `nfs` keeps the render unchanged. ([#939](https://github.com/adanalife/tripbot/pull/939))
- **Prod serves the dashcam corpus from the minipc's local NVMe, not the NAS.** prod-1 `dashcam_source` is set to `local`, so prod's vlc instances mount the node-local `vlc-dashcam-local` PVC instead of the NFS-backed `vlc-dashcam`. `dashcam_source=nfs` remains the instant fallback. Requires the local copy to be populated first ([infra#773](https://github.com/adanalife/infra/pull/773/changes)). ([#941](https://github.com/adanalife/tripbot/pull/941))

### CI / Tooling

- **Changelog entries are now [towncrier](https://towncrier.readthedocs.io) fragments.** Each PR drops a file in `changelog.d/` (e.g. `889.fix.md`) describing its change, instead of editing `CHANGELOG.md`'s `[Unreleased]` block; `task changelog:build VERSION=x.y.z` collates them into a release section at cut time. A `changelog` CI check requires a fragment on every PR into `develop` (escape with the `skip-changelog` label), so the release changelog stays complete by construction — and parallel PRs no longer conflict on `CHANGELOG.md`.
- **Commit messages are now checked against [Conventional Commits](https://www.conventionalcommits.org).** A `conventional-pre-commit` hook at the `commit-msg` stage rejects commit subjects that don't follow `type(scope): summary`, keeping history machine-parseable for the towncrier fragments and future release automation. `pre-commit install` now wires up both the `pre-commit` and `commit-msg` hook types.
- **PR titles are now checked against [Conventional Commits](https://www.conventionalcommits.org) in CI.** A `pr-title` workflow (`amannn/action-semantic-pull-request`) fails a develop-targeting PR whose title isn't `type(scope): summary`. This closes the gap where the local `commit-msg` hook couldn't reach the squash subject — since develop PRs squash-merge, the PR title is what lands in history. Release PRs to `master` are exempt (they merge-commit, so the `Release vX.Y.Z` title never becomes a commit subject).
- **The arm64 release build legs now run on a self-hosted runner on the rpi5 when it's available.** `release.yml` and `release-development.yml` gained a `pick-runner` job that probes for the `arc-arm64-tripbot` runner (Actions Runner Controller on the mini-PC's Raspberry Pi) and routes the tripbot/vlc/onscreens arm64 builds to it, falling back to GitHub-hosted `ubuntu-24.04-arm` automatically when the Pi is offline. Cuts the 2×-billed GitHub-hosted arm minutes that were the main GHA-budget drain. `obs` stays GitHub-hosted for now (its heavy compile isn't on the Pi yet).
- **The standing pending-release PR previews the assembled release notes via towncrier.** Its body now renders `towncrier build --draft` from the queued `changelog.d/` fragments, and bases the suggested-version-bump hint on the fragment types as well as commit prefixes — replacing the old scrape of the `[Unreleased]` changelog section.

### Cleanup

- **Removed tripbot's in-process YouTube auth + Data-API client.** YouTube OAuth and runtime now live entirely on the platform-gateway (`gateway-youtube`) — tripbot holds no YouTube token at any point. Deletes `pkg/youtube`, `cmd/youtube-chat-spike`, the YouTube legs of the `/auth` handlers, the in-process chat send/poll path, and the `YOUTUBE_CLIENT_ID` / `YOUTUBE_CLIENT_SECRET` / `YOUTUBE_CHANNEL_ID` config. `YOUTUBE_API_URL` is now required on a `PLATFORM=youtube` instance; without it the instance comes up with no YouTube chat (logged loudly), the same stay-up contract as a missing Twitch token. ([#940](https://github.com/adanalife/tripbot/pull/940))

### Misc

- **The NATS subjects + event envelopes are now emitted as a machine-readable registry (`pkg/contract/eventbus.json`).** `go generate ./pkg/contract` writes the subjects, transports, JetStream stream names, and per-payload JSON Schemas (derived from `pkg/eventbus`), so consumers like tripbot-console can sync one file to discover the wire format instead of hand-rebuilding subject strings + field names. A reflection test asserts the registry matches the real structs, so it can't drift.

## [v3.8.0] — 2026-06-20

Minor release. Completes phase 3b of the platform-gateway migration: with the `chatbot.twitch_gateway` flag on, every in-process Helix caller — OBS watchdog live-check, broadcaster chat-send, cached audience refresh, and EventSub channel-ID resolution — now routes through the standalone gateway, making it the single Helix caller (the prerequisite for moving the Twitch token out of tripbot). Adds the YouTube outbound-chat-send analog behind its own flag, plus cross-service trace propagation from the gateway client. Rounds out with a `!km <username>` fix and two stage streaming-pipeline fixes: re-enabling VAAPI iGPU encode for obs-youtube and co-locating the vlc/onscreens feeders with their OBS pod to stop cross-node stutter.

### Platform gateway

- **The gateway is the single Helix caller when `chatbot.twitch_gateway` is on.** Phase 3b routed the remaining in-process Helix callers through the standalone platform-gateway: the OBS watchdog live-check and the broadcaster chat-send, via a new shared `pkg/gateway` client ([#920]); and the cached audience refresh — the subscriber/follower-count pollers, chatter refresh, and the live follower check ([#921]). The hot path stays a local cache read (only the *refresh* is repointed at the gateway), and the in-process paths remain as the flag-off fallback. ([#920], [#921])
- **EventSub keeps its `ChannelID` under the gateway.** `channelID` was only ever populated as a side effect of an in-process Helix call; once 3b routed those through the gateway, it stayed empty and EventSub was silently skipped — no new-follower / new-subscriber announcements. `startEventSub` now resolves the channel ID via the gateway when the flag is on, falling through to the existing skip-with-warning on error. ([#922])
- **YouTube outbound chat-send can route through `gateway-youtube`.** The YouTube analog of the Twitch cutover — `tripbot-youtube`'s send dispatches through the gateway behind a two-layer gate (`YOUTUBE_API_URL` wired + the `chatbot.youtube_gateway` flag), defaulting off with in-process failover. The inbound chat poll stays in-process. Migration 023 seeds the disabled flag. ([#925])
- **Cross-service trace propagation from the gateway client.** The gateway HTTP client's transport is wrapped with `otelhttp`, so it starts a client span and injects the W3C `traceparent` header; the gateway nests its server span under tripbot's, so a chat command and the Helix call it triggers form one cross-service trace. Inert when tracing is disabled. ([#924])

### Onscreens

- **Rotator overlays no longer go blank in OBS after their first rotation.** The left/right rotators centered text with `position:absolute` + `transform`, promoting it to its own compositing layer that OBS's offscreen renderer (CEF OSR) captured once but failed to repaint on later rotations. Switched to the same normal-flow, `margin-left`-offset centering middle-text uses, so OSR repaints it correctly. (Surfaced when #885 moved the rotators to `innerHTML` swaps on the composited layer.) ([#916])

### Fixes

- **`!km <username>` shows the named user's kilometres.** It previously ignored its argument and always reported the *caller's* km; it now mirrors `!miles`'s other-user lookup — strips a leading `@`, looks the user up via `Sessions.Find`, and reports their distance (same "I don't know them, sorry!" fallback). With no arg it still falls back to the caller. ([#889])

### Deploy / Infra

- **VAAPI iGPU encode re-enabled for the stage YouTube stream.** stage-1 `obs-youtube` moves back onto the MS-01's Iris Xe (`ffmpeg_vaapi_tex`, quality `high`) via the `gpu.intel.com/i915` claim instead of saturating the rpi5 with software x264. Concurrent VAAPI encoders are back to 2 (prod obs-twitch + stage obs-youtube), within the budget that only stuttered at 3. ([#923])
- **Stage vlc/onscreens feeders co-locate with their OBS pod.** Their independent rpi5 node-affinity is replaced with a preferred `podAffinity` anchoring them to their platform's OBS pod, so the continuous video feed + overlays stay on localhost instead of crossing the WiFi link to reach OBS — the cause of choppy stage `obs-youtube`. Scoped to stage; prod and local unchanged. ([#926])

### CI / Tooling

- **Back-merge PR title shows the correct release version.** `backmerge.yml` read the version via `git describe`, which races `auto-tag.yml` on the same master push and labeled the back-merge PR with the *previous* release (e.g. v3.6.0 right after v3.7.0 shipped); it now reads the just-released version from the CHANGELOG, which is race-free. ([#919])

## [v3.7.0] — 2026-06-19

Minor release. Lands the groundwork for routing the Twitch bot's command-time Helix calls through the standalone platform-gateway (staged on stage-1, off by default), credits the triggering viewer on the timewarp overlay, adds a console-facing feature-flag API, and automates the develop↔master release flow. Plus fixes to vlc-server resume-on-restart, the auth-bootstrap retry path, and the `:develop` image-rebuild filter for migrations.

### Onscreens

- **The triggering viewer is credited on the timewarp overlay.** When a viewer runs `!timewarp` or guesses the state correctly with `!guess`, their username now appears as a credit line (e.g. `@viewer`) under the **TIMEWARP** wordmark on the full-screen warp overlay, carried end-to-end over the existing onscreens NATS command surface. ([#888])

### Console / API

- **`GET /api/flags` + `POST /api/flags/{key}`** give the standalone console a read/toggle surface over tripbot's feature flags (the console has no DB access, so it proxies). `GET` returns each flag's key/description/enabled state and targeting; `POST {"enabled": bool}` flips the global default via `feature.FlagToggler`. The flag client is injected into `Server` before the HTTP server starts; if Postgres is unavailable `GET` reports `ok:false` and `POST` returns 503. ([#903])

### Platform gateway

- **The chatbot's Twitch Helix surface can route through the platform-gateway.** `App.Twitch` picks its adapter by config — `TWITCH_API_URL` set → an HTTP client against the `gateway-twitch` service; empty → the in-process `pkg/twitch` path (the zero-config default, so existing envs are unchanged). No command code changed, cashing in the `App.Twitch` injection seam. ([#904])
- **`chatbot.twitch_gateway` feature flag seeded (disabled).** Migration `021` creates the runtime kill-switch row for the gateway routing so it's toggleable from the console (`feature.SetEnabled` only UPDATEs, so the row had to exist first). ([#906])
- **Stage twitch stack re-enabled behind the gateway, manually scaled.** `stage-1` adds `twitch` to its platforms and points `stage-1-tripbot-twitch` at `gateway-twitch` via `TWITCH_API_URL`; only the twitch instance gets it (youtube + prod stay in-process). A stage-only `manual_replicas` omits `spec.replicas` so Argo never resets a hand/console scale; prod keeps `replicas: 1`. ([#905], [#911])

### Fixes

- **vlc-server actually resumes its last-played clip on restart.** libvlc 3.0.x's `libvlc_media_list_player_play_item_at_index` reads the player's *existing* media before swapping in the requested one and returns `-1` when that prior media is nil — i.e. on the very first play against a freshly-created list player — even though playback actually starts. So the first startup play (resume-from-marker / resume-from-lastplayed) saw a spurious "cannot play the requested media", fell through to `PlayRandom`, and a restart almost never resumed where it left off. The media player is now primed with the first loaded clip at construction, so the first real play returns correctly and resume-on-restart lands on the right clip with the position seek following. ([#907])
- **`auth-bootstrap` stays up for a retry when the wrong Twitch account signs in.** It used to `log.Fatalf` on an identity mismatch, killing the job pod and dropping the `kubectl port-forward` so the bootstrap chain moved on. It now keeps the listener (and port-forward) up, re-surfaces the authorize URL to re-auth in place, and only writes the token + exits 0 on a matching identity. Still bounded by `flowTimeout`. ([#892])

### CI / Tooling

- **Standing draft release PR.** A new `pending-release.yml` keeps one draft `develop → master` PR open showing the next release's diff, `[Unreleased]` changelog, queued commits, and a suggested version bump — so the bump decision is made against a real diff instead of up front. ([#909])
- **Automated master → develop back-merge PR.** A new `backmerge.yml` opens/refreshes a standing PR merging master back into develop after each ship, so the two branches don't diverge (merge it with a merge commit, never squash). ([#913])
- **The `:develop` tripbot image rebuilds on `db/migrate/**` changes.** Migrations are baked into the image and applied by its `migrate` initContainer, but the per-image path filter omitted `db/migrate/**`, so migration-only PRs silently never shipped (e.g. #906's flag seed). ([#908])
- **Repaired orphaned `pkg/server` profile tests** left after the admin-panel removal. ([#902])
- **Bumped `actions/checkout` from 6 to 7.** ([#901])

## [v3.6.0] — 2026-06-19

Minor release. Retires the in-tripbot admin panel now that the standalone tripbot-console has taken over, leaving tripbot with just the HTTP surface the console and operators still depend on. Also renders inline markdown on the text overlays (so `!command` refs show in monospace), fixes the events table being frozen at year-0001 timestamps, exposes the prod dashcam feed over an RTSP NodePort for off-cluster pulls, and moves stage's software-encoder OBS onto the Pi 5 worker.

### Cleanup

- **Remove the in-tripbot admin panel in favor of tripbot-console.** Now that the standalone tripbot-console covers the admin dashboard, the in-process panel and its live-console SSE hub retire (`admin.go`, `hub.go`, `events.go`, the chat-send publisher form, `somafm.go`, `authcard.go`, and the vendored htmx/leaflet/sse assets). The HTTP surface the console and operators still need stays: `/auth/init` + `/auth/callback` (now fronted by a minimal landing page at `/` linking the bot/broadcaster/YouTube login flows), the read-only `/api/user`, `/api/chatters`, `/api/db/migration`, and `/admin/map/corpus` endpoints the console proxies over the in-namespace Service, plus `/version`, `/health`, and `/metrics`. The `chat.send` NATS subscriber stays in cmd/tripbot (the Twitch-identity owner), ready for the console to publish to once its chat-send feature lands. ([#886])

### Onscreens

- **Inline markdown on text overlays.** The text onscreens now render `` `code` ``, `**bold**`, and `*italic*` — the motivating case being monospace `!command` references on the middle-text overlay and the bottom-strip rotators. A dependency-free `renderInlineMarkdown` (stdlib only) converts a small marker subset at the `state.json` wire boundary, so the stored content (and the JetStream-persisted middle-text state) stays raw markdown and only the served copy is HTML; code spans win over emphasis so asterisks inside backticks stay literal. Enabled on `middle-text`, `left-message`, and `right-message`, with a monospace pill style for legibility over the dashcam video. ([#885])

### Deploy

- **Prod vlc RTSP exposed via a fixed NodePort (30854) for off-cluster pulls.** A LAN box — e.g. OBS on a desktop — can now pull the raw dashcam feed at `rtsp://<minipc-ip>:30854/dashcam` (use `rtsp_transport=tcp`) without kubectl or a port-forward, the NodePort analogue of the k3d-only `<name>-host` LoadBalancer convenience (the minipc has no LB controller). A new `vlc_rtsp_node_port` knob defaults to `0` (no NodePort) and is set only on `prod-1`'s `twitch` instance; VNC/HTTP stay in-cluster. Overlays are composited in OBS, so the off-cluster feed is raw video only. ([#896])
- **Stage software-encoder OBS scheduled onto the rpi5 worker.** Stage `obs-youtube` already runs software x264 (no iGPU claim, the 2026-06-15 VAAPI-contention cap), so it now biases toward the ephemeral arm64 `adanalife-rpi5` worker via a toleration + preferred node affinity — offloading the x264 encode off the MS-01 and easing the recurring stream/pipeline CPU contention. Gated on `prefer_rpi5 and not (gpu and obs_gpu)`, so only a software-encoder OBS biases toward the Pi; the affinity stays preferred (never required) so OBS recovers onto the MS-01 if the Pi is unplugged. Prod VAAPI OBS is untouched. ([#884])

### Fixes

- **`date_created` is stamped on insert (events table was frozen at `0001-01-01`).** Since 2026-05-15, every `events` row (and `users`/`scoreboards`/`scores`/runtime-created `videos`) was written with a year-1 timestamp: the GORM migration (#499) replaced raw INSERTs that omitted `date_created` (letting the column default apply) with `Create(&struct{})`, and GORM writes the zero-value `time.Time` unless the field carries a `default`/`autoCreateTime` tag. This is why the events table looked frozen — rows inserted fine but date filters saw nothing recent (`SessionCount` has no date filter, so miles stayed healthy and masked it). The create-time columns now carry `gorm:"autoCreateTime"`, and the user-profile popover computes a best-effort first-seen from the earliest non-sentinel event for accounts caught in the bug window. Already-written `0001-01-01` rows are not back-filled (unrecoverable); new rows are correct from here forward. ([#887])

## [v3.5.0] — 2026-06-15

Minor release. Rounds out the per-platform YouTube stream: a public `!carsound` command to cycle a set of license-clean background-audio voicings, platform-scoped eventbus payloads so the two bot instances stop clobbering each other's now-playing/viewer state, and two new console-facing JSON endpoints. Also persists the middle-text overlay across restarts, labels the monthly leaderboard with the month name, fixes the corpus map drawing cross-country streaks, and shuffles GPU/scheduling on stage to relieve iGPU contention.

### Chat

- **`!carsound` (alias `!carhum`) — public, YouTube-only — cycles background-audio voicings.** Builds on the v3.4.0 Car Hum bed with four character presets (`idle`, `highway`, `backroad`, `mountain`), each rendered at build time to a seamless-looping FLAC (numpy/scipy live only in a throwaway Docker stage, so the runtime image stays small and boot stays network-free). No arg reports what's playing; `next` cycles, `<name>` jumps, `list` shows options. Repoints the OBS `Car Hum` source live over the WebSocket — no scene reload — and increments `tripbot_carsound_selections_total{sound=…}` so the most-played voicing is rankable. Also centralizes per-platform command scoping behind a `Command.Platforms` field instead of the implicit "unset == Twitch" assumption. ([#869])
- **`!guesslb` registered as an alias for `!guessleaderboard`.** ([#865])

### Onscreens

- **Middle-text overlay survives an onscreens-server restart.** The text previously lived only in memory, so a restart blanked the OBS browser source. It's now backed by a `MaxMsgsPerSubject=1` JetStream last-value cache (`TRIPBOT_ONSCREENS_MIDDLE`), mirroring the vlc `lastplayed` pattern, and restored on startup. Degrades gracefully when NATS is unavailable. ([#861])
- **Monthly-miles leaderboard overlay is labeled with the current month** (e.g. "June Miles") instead of a generic "Monthly Miles" header, via a new `scoreboards.CurrentMilesMonth()` sharing the board's `time.Now()` basis. Lifetime and guess boards untouched. ([#866])

### Console / API

- **`GET /api/chatters`** returns the logins currently in chat as sorted JSON (`{"chatters": […], "count": N}`), read from the in-process chatter set the `UpdateSession` cron already refreshes — no new scope, no request-time network call. Feeds the standalone tripbot-console's active-chatters panel. ([#868])
- **`GET /api/db/migration`** reports the current golang-migrate schema version as JSON (`{"ok": true, "version": 20, "dirty": false}`) so the console (which holds no DB access) can surface which migration each env's Postgres is on. ([#867])

### Eventbus

- **`video.changed` and `viewers.count` payloads carry a `Platform` field.** With both `tripbot-twitch` and `tripbot-youtube` publishing to the same env-scoped subjects, the two instances were clobbering each other — the console's now-playing card, map trail, and viewer count flickered between platforms. Mirrors the existing `ChatMessage.Platform` treatment so the console can render per-platform state; `omitempty` keeps pre-upgrade events graceful. ([#871])

### Deploy

- **vlc dials its own platform's OBS.** `OBS_WEBSOCKET_ADDR` is now set to `obs-<platform>:4455` on every vlc instance, fixing `vlc-youtube` dialing the nonexistent `obs-twitch` (it had been falling back to the baked-in default). Twitch now carries the address explicitly too rather than leaning on the default. One-time `prod-1-vlc-twitch` rollout from the config-hash bump (value unchanged). ([#877])
- **Config-driven OBS streaming toggle; stage YouTube turned on durably.** Replaces the hardcoded `env == prod-1 and platform == twitch` streaming special-case with a per-env `obs_streaming` config tuple, and makes `stage-1-obs-youtube` stream — its stream key now ESO-managed from Secrets Manager (`k8s/obs/youtube-stream-key`) as the single source of truth. Prod renders byte-identical. ([#870])
- **Stage `obs-youtube` drops its iGPU claim and software-encodes** (x264) to relieve the three-way VAAPI contention on the single mini-PC Iris Xe that stuttered the prod twitch stream. Adds an `obs_gpu` knob mirroring `vlc_gpu`. Stopgap until the optimization job stops needing the iGPU; prod untouched and CPU-protected by its priority class. ([#875])
- **Stage stateless apps prefer the ephemeral rpi5 worker when present.** `tripbot`/`vlc`/`onscreens` opt into a toleration + preferred (never required) node affinity toward the Pi 5, falling back to the MS-01 when it's unplugged. OBS opts out (no H.264 hardware encoder); prod untouched. ([#876])

### Fixes

- **Corpus map no longer streaks across the country.** The admin map's full-route overlay drew one continuous polyline through every clip in film order, so each new trip (van resuming thousands of miles away) drew a straight line across the map. `mapCorpusHandler` now splits the route into segments on gaps over 25mi, returning a nested array Leaflet renders as a multi-polyline — no JS change. The fix lives in the durable `/admin/map/corpus` endpoint so the console's eventual map inherits it. ([#864])

### Tooling

- **Removed the `cv:stats` Taskfile target** — the last video-pipeline producer-era trace in tripbot, now that embedding production/monitoring lives in the standalone `video-pipeline` repo. A full audit confirmed nothing else producer-era remained. ([#863])

## [v3.4.1] — 2026-06-14

Patch release. A one-time cleanup pass over the dashcam GPS corpus: adds a provenance column so synthesized fixes are distinguishable from real OCR ones, ships a `backfill-coords` tool that interpolates missing fixes and corrects digit-flip OCR outliers, and reseeds `videos.csv` with the corrected coordinates. Also makes prod-deploy impact visible on PRs. No runtime behavior change.

### CI

- **Prod-deploy impact is now visible on PRs.** A new `prod-dist-warning` workflow posts a sticky comment on any PR into `master` that touches a prod app deploy unit (`cdk8s/dist/prod-1-*-twitch.k8s.yaml`), spelling out that merging deploys to prod — the gap that let an incidental manifest change go live on a release merge rather than a deliberate gesture. The version-bump PR template (`bump-prs.yml`) is also corrected to be component-aware: prod-1 apps autosync from master so merging *is* the deploy, except OBS which is held out of autosync and still needs a manual sync. ([#859])

### Database

- **`videos.coord_source` provenance column.** Each clip's stored GPS fix now records how it was derived (`ocr`/`interpolated`/`rejected`/`missing`) so a corrective pass can mark synthesized points and a future re-OCR won't mistake them for real fixes. Existing 0/0 and flagged rows backfill to `missing`; runtime-created clips are stamped `missing` on save. ([#846])

### Tooling

- **`cmd/backfill-coords` cleans up dashcam GPS coordinates.** Walks videos in film order and, per the new provenance column, interpolates missing fixes from in-session neighbours and replaces digit-flip OCR outliers with interpolated points. Conservative by design — only judges clips against neighbours inside the interpolation window (trip boundaries left alone) and never clears a coordinate it can't replace. Dry-run by default; `--apply` writes to the DB, `--output-sql` emits idempotent slug-keyed UPDATEs. Mirrors `cmd/backfill-miles`. Removes the dead `collect-gps` script (its OCR pass was retired in #79). ([#846])

### Seed

- **Reseed `videos.csv` with corrected coordinates.** 334 clips gain coordinates (81 replacing digit-flip OCR outliers, 253 filling missing fixes), captured from `backfill-coords` output so a fresh seed loads the cleaned corpus. ([#846])

## [v3.4.0] — 2026-06-14

Minor release. Headlined by a license-clean synthesized background-audio bed for the YouTube stream; also drops vlc-server's unused iGPU claim to ease co-tenant contention, plus routine OpenTelemetry dependency bumps.

### OBS

- **YouTube scene plays a synthesized "Car Hum" bed in place of SomaFM.** SomaFM (the "Groove Salad Classic" source) is already stripped on YouTube to dodge Content ID strikes, leaving that stream with no background audio. A new license-clean, locally-generated car-interior drone (`assets/car-hum-loop.flac`, a seamless 4-min loop produced by `script/carhum/`) is added to the shared scene as the "Car Hum" `ffmpeg_source` and now fills that gap. The per-platform strip in the OBS entrypoint is symmetric: YouTube keeps Car Hum and drops SomaFM, Twitch keeps SomaFM and drops Car Hum, so the two never play together. ([#854])

### Deploy

- **vlc-server no longer claims the iGPU on prod or stage.** vlc does stream-copy plus trivial software decode and doesn't need the iGPU — proven live on stage (CPU flat at ~0.04 cores with and without `/dev/dri`, 0 restarts). Dropping its i915 claim frees an iGPU slot and eases the co-tenant contention behind the 2026-06-11 prod-stutter incident; OBS keeps its iGPU for VAAPI encode. Re-introduces the `vlc_gpu` flag (gating the claim on `gpu and vlc_gpu`) that the cdk8s-into-repo cutover had silently dropped, set `False` on both envs. ([#853])

### Dependencies

- Bump `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp`. ([#761])
- Bump `go.opentelemetry.io/otel/sdk/log` from 0.19.0 to 0.20.0. ([#762])
- Bump `go.opentelemetry.io/otel/exporters/prometheus`. ([#763])

## [v3.3.4] — 2026-06-14

Patch release. CI/release-plumbing only — no runtime behavior change. Finishes wiring the in-repo prod bump-PR workflow into the release flow now that the cdk8s manifests live in this repo.

### CI

- **Release workflow fans out the prod bump PRs in-repo.** The release's final step now triggers this repo's `bump-prs` workflow (`workflow_dispatch` with the released version) instead of a cross-repo `repository_dispatch` to infra, matching the cdk8s manifests' move into this repo. Still mints the `adanalife-automation` App token so the bump commits re-fire the cdk8s synth gate — now scoped to this repo. ([#845])
- **Bump-PR commits and titles marked `#none` so they don't fire releases.** Bump PRs target `master`, where every push auto-tags and triggers a release; a pin edit is a deploy gesture, not a release, so each merged bump was minting a spurious release. The `#none` skip-tag marker (honored by `anothrNick/github-tag-action`) now lives in both the commit message and the PR title, surviving whether the PR is merge-committed or squash-merged. ([#851])

## [v3.3.2] — 2026-06-13

Patch release. Build/deploy plumbing only — no runtime behavior change. The Kubernetes manifest authoring for this repo's four images now lives in-repo, and the per-component prod-pin bump workflow moves here alongside it.

### Deploy

- **In-repo cdk8s authoring layer for the app manifests.** The Kubernetes manifests for the images built from this repo (tripbot, vlc, onscreens, obs) are now authored as typed cdk8s constructs under `cdk8s/`, synthesized to committed `dist/` files that Argo delivers cross-repo. Ported from `infra/cdk8s` byte-identically; identity-level Secrets and the prod-stream PriorityClass/ResourceQuota move into a per-env `tripbot-identity` deploy unit. The stateful + shared-platform layer (postgres, ESO store, observability, cert-manager, dashcam PV, Argo config) stays in infra. Adds a verify-only synth gate, pytest checks, and `cdk8s:*` Taskfile targets. ([#840])

### CI

- **Per-component prod bump-prs workflow relocated into this repo.** Now that `cdk8s/versions.yaml` lives here, the prod-pin bump workflow moves over too. On a release it opens one prod-pin bump PR per component against `master`, carrying the pin edit and re-synthed `dist/` in the same commit. `workflow_dispatch`-only for now — wiring `release.yml` to fire it happens together with the infra Argo cutover. ([#841])

## [v3.3.1] — 2026-06-13

Patch release. Mostly YouTube platform-awareness polish on the chat and overlay surfaces, plus a read-only user-profile endpoint for the standalone console and a copyright-safety fix for the YouTube OBS scene.

### chatbot

- **Chat command surface is YouTube-aware.** `!state` and `!location` are enabled on YouTube — they read the current video's location/flag overlay, the same read-current-video-state category as the already-allowlisted `!weather`/`!time`/`!date`/`!sunset` (they previously no-op'd silently as unindexed commands on YouTube). `!commands` and the rotating `!help`/Chatter lines now build from a featured set filtered by the platform's indexed lookups, so a YouTube instance no longer advertises commands that aren't in its allowlist (`!guess`, `!miles`, `!leaderboard`, `!song`). ([#835])

### onscreens

- **Periodic leaderboard overlay rotates across all three boards.** The 5-minute cron job only ever showed the guess leaderboard; it now picks at random each tick — monthly miles and guesses weighted evenly, lifetime total miles rarely (5%), with an empty pick falling back to monthly miles. Shared leaderboard row helpers move into `pkg/scoreboards`, and the job moves onto the chatbot `App` so it reaches the overlay and lifetime leaderboard through injected interfaces. ([#834])
- **Rotator overlays are platform-aware with weighted selection.** The left/right overlay rotators advertised commands regardless of platform, so a YouTube overlay promoted `!miles`/`!guess` (disabled there). A new `rotatorMessage{Text, Platforms, Weight}` scopes lines to streaming platforms and biases weighted-random selection, replacing the old duplicate-line likelihood trick; `!miles`/`!guess` lines are scoped to Twitch. Adds `ONSCREENS_SERVER_PLATFORM` config. ([#836])

### server

- **Read-only JSON user-profile endpoint for tripbot-console.** `GET /api/user/{username}` returns the same chatter stats as the in-tripbot `/admin/user` popover (miles, monthly miles, sessions, first/last seen) as JSON, so the standalone console — which has no DB access by design — can proxy here over the in-namespace Service. ([#837])

### obs

- **YouTube OBS scene strips the SomaFM audio source.** The "Groove Salad Classic" ffmpeg_source plays licensed SomaFM audio that trips YouTube's Content ID and earns copyright strikes. Twitch tolerates it, so the OBS entrypoint now strips the source (and its scene-item references) via a jq pass over the rendered `Tripbot.json` only when `STREAM_PLATFORM=youtube`. Adds jq to both the amd64 and arm64 OBS images. ([#838])

## [v3.3.0] — 2026-06-12

Minor release. The headline is **typo-tolerant chat commands** — misspelled `!`-commands now fuzzy-route to their nearest trigger, bare state names work as guess shortcuts (`!florida`), and the 31 hand-registered typo aliases that accumulated over the years retire. Alongside it: vlc-server resumes its last-played video (and position) across restarts, the shared database gains a platform dimension so YouTube viewers and feature flags don't collide with Twitch's, and each bot instance publishes auth-token snapshots to NATS for the standalone admin console.

### chatbot

- **Misspelled commands fuzzy-match to their nearest trigger.** When an unrecognized `!`-command arrives, `findCommand` falls back to Levenshtein matching against the registered triggers and aliases, so `!locaiton` runs `!location` — no per-typo alias maintenance. The edit-distance threshold scales with length (1 edit for 4–6 runes, 2 for 7+; under 4 never matches), ties refuse to guess (`!tate` is one edit from both `!date` and `!state`, so it routes to neither), bare-word triggers don't participate, and every fuzzy route logs the typed text + resolved trigger so mis-routes are auditable. Routing goes through normal dispatch, so follow/subscriber gates and per-platform allowlists still apply. ([#827])
- **State-name guess shortcut.** A `!`-prefixed US state or territory name runs `!guess` with that state — `!florida`, `!new york` (multi-word assembles from params), trailing chatter dropped. Full names only: two-letter abbreviations are excluded so `!hi`/`!ok`/`!me` can't fire accidental guesses. Registered commands always win, and the shortcut resolves `!guess` through the platform-filtered lookup, so it's inert where `!guess` isn't allowlisted. Runs ahead of the fuzzy fallback, so an exact state name can't be stolen by a near-miss trigger. ([#829])
- **Hand-registered typo aliases retire.** The 31 typo aliases in the command registry (`!locaiton` cousins, `!guss` variants, `¡`-prefixed forms, …) are removed — each verified to still resolve via the fuzzy router or prefix normalization. `normalizeCommandPrefix` also gains digit-1 handling (`1` is the unshifted `!`), rewriting a leading `1` when a letter follows, which gives every command its `1`-prefix form for free. Two aliases beyond the fuzzy router's edit caps stay, with comments saying why. ([#831])

### vlc

- **vlc-server resumes its last-played video across restarts.** Every successful play publishes file + position to a per-platform `tripbot.<env>.vlc.lastplayed.<platform>` subject backed by a new `TRIPBOT_VLC_LASTPLAYED` JetStream last-value cache (a 5s ticker keeps the position current). On startup the pick order is watchdog file marker → JetStream lastplayed → `PlayRandom`, each step degrading gracefully, and position resume waits for libvlc to reach Playing before seeking (skipping seeks that land in the clip's final 2s). The per-platform leaf matters because the twitch and youtube vlc instances share the one per-env NATS. `VlcServerConfig` gains `STREAM_PLATFORM`, stamped by the cdk8s vlc factory ([infra #717]). ([#830])

### youtube

- **Platform-scoped users, events, and feature flags.** Gets the shared per-env Postgres ready for the YouTube bot going live. Migration 018 adds a `platform` column (default `twitch`) to `users` + `events`, with `users` uniqueness becoming `(platform, username)` so a YouTube viewer named `foo` doesn't merge with the Twitch `foo`; migration 019 platform-scopes `feature_flags` with a `(key, platform)` composite PK and seeds YouTube copies of the existing flags, so enabling a flag on YouTube no longer enables it on Twitch. All queries scope by the instance's configured platform; existing single-platform behavior is unchanged. ([#832])

### pubsub

- **Auth-token snapshots publish to NATS for the standalone console.** Each platform instance publishes its token state to `tripbot.<env>.auth.status.<platform>` every 30s, backed by a new `TRIPBOT_AUTH` JetStream stream with `MaxMsgsPerSubject: 1` — a last-value cache, so a freshly-connected console replays exactly the latest snapshot per platform and then receives live updates. The envelope carries account, login-as, expiry, missing/expired reason, and the absolute `/auth/init` URL; re-auth itself stays in tripbot. The in-process admin hub is unchanged. ([#826])

### CI

- **`python:3.13-slim-bookworm` joins the GHCR mirror refresh.** The base image for the standalone tripbot-console service is mirrored to `ghcr.io/adanalife/mirror/python` and added to the weekly refresh list, same as the Go/Ubuntu/migrate bases. ([#828])

### docs

- **Docs sweep: self-contained comments, CHANGELOG cleanup, README pruning.** Code/Taskfile/workflow comments no longer point at docs a reader of this repo can't follow — load-bearing context is stated inline, history-narration trimmed, a few stale comments corrected ([#823]). The CHANGELOG is synced from master and swept for internal milestone jargon ([#824]). Three obsolete READMEs (`infra/`, `infra/certs/`, `db/`) are removed and the root README's dead links fixed ([#825]).

## [v3.2.1] — 2026-06-11

Patch release. The v3.2.0 release run pushed its per-arch images but died at the version-stamping verify step — Docker Hub rate-limited the pull-back of our own just-pushed images — so the multi-arch manifests, GitHub Release, and infra bump dispatch never happened. This release fixes the verify step and ships everything v3.2.0 built.

### CI

- **Release builds load images locally for verification.** Each per-arch build leg now passes `load: true` alongside `push: true` (multi-exporter, buildx ≥ 0.13), so the verify step runs the locally-built image instead of pulling it back from Docker Hub — closing the "our own images still count against the pull quota" gap the GHCR mirrors ([#820]) deliberately left open. ([#821])

## [v3.2.0] — 2026-06-11

Minor release. The headline is the **YouTube provider going code-complete**: a `PLATFORM=youtube` tripbot instance now runs end to end — channel-owner OAuth, outbound live-chat sends, an inbound chat poller, and a boot sequence that branches per platform — built on the provider-neutral chat seams the no-globals refactor left behind. Alongside it: release CI now publishes a GitHub Release per tag, dispatches the infra version-bump PRs automatically, and pulls base images from GHCR mirrors to dodge Docker Hub rate limits; and `!report` validates its Discord webhook URL before POSTing.

### youtube

- **Per-platform command allowlist.** A `Platform` selector on `TripbotConfig` / `chatbot.App` (read from the same `STREAM_PLATFORM` key the OBS image uses): Twitch runs the full registry, while a YouTube instance indexes only a stateless v1 allowlist — info, weather/time, playback control, and socials — excluding everything that needs per-user identity, miles, Helix, or admin. Disabled triggers stay registered but never index into dispatch. ([#809])
- **Quota spike.** A standalone `cmd` proving the OAuth → active-broadcast → `liveChatId` → poll/insert loop and measuring Data-API quota burn at the server-suggested cadence, before any tripbot wiring. Zero `tripbot/pkg` imports per the package-boundary discipline. ([#811])
- **`pkg/youtube` — channel-owner OAuth + token storage.** Owns tripbot's YouTube identity, mirroring `pkg/twitch`. One identity (no bot/broadcaster split — YouTube live chat must run as the channel owner), stored in `oauth_tokens` as `provider=youtube` keyed by channel ID, with lazy refresh persisted on rotation and an optional `YOUTUBE_CHANNEL_ID` pin that rejects consent from the wrong channel before persist. Admin re-auth is `/auth/init?account=youtube` riding the existing callback path. All optional — a Twitch instance with no `YOUTUBE_*` env never fatals. ([#812])
- **Outbound live-chat client.** `youtubeChat` implements the `ChatClient` seam: `Say` posts via `liveChatMessages.insert` (truncating at YouTube's 200-char limit, stripping the IRC `/me` prefix), `Whisper` is a no-op, and a rebindable `liveChatBinding` shared with the inbound poller drops sends with a log while no broadcast is live. Output flows through the console mirror, so the admin live console sees YouTube exactly like Twitch. ([#814])
- **Inbound live-chat poller.** Discovers the active broadcast (idling quietly while not live), binds its chat, and pages messages into the shared command path via `HandleYouTubeMessage` — identical to the Twitch handler except viewers get a transient, never-persisted `users.User` (v1 punts identity/presence/miles). Skips the backlog page on every bind so stale commands never replay, filters the bot's own echoes, and disambiguates quota-exhausted 403s (5-min backoff) from chat-ended 403s (rediscover). ([#815])
- **Per-platform boot.** `Run()` branches on `STREAM_PLATFORM`: the spine (telemetry, admin server, player, cron, flags, NATS, event hub) stays common; only chat bring-up swaps. A youtube pod with no token stays Ready and retries every 15s so the admin panel's re-auth flow is always reachable. Twitch-only steps (IRC token refresh, EventSub, follower/subscriber polls, chatter sessions, Discord, the OBS watchdog, the admin `chat.send` subscriber) are gated off non-Twitch instances, and `eventbus.ChatMessage` gains a `platform` tag so the admin console can disambiguate the two instances' chat lines. ([#816])

### CI

- **A GitHub Release per release tag.** `release.yml` gains a `github-release` job after the image manifests publish: `gh release create --generate-notes --verify-tag`, giving each version a human-readable PR-by-PR summary (the Releases page had been frozen at v1.9.1). ([#817])
- **Release dispatches infra bump PRs.** Once all four manifests publish, the workflow fires a `tripbot-release` `repository_dispatch` at `adanalife/infra`, whose bump-prs workflow ([infra #694]) fans out one "bump prod \<component\>" PR per image — merge = deploy. Authenticated via a short-lived adanalife-automation GitHub App token ([infra #695]); no PAT. ([#818])
- **Base images pull from GHCR mirrors, not Docker Hub.** Docker Hub's pull rate limit broke CI twice in one day; the 429 hits at manifest resolution, before the GHA layer cache is ever consulted. The three third-party base images (`golang`, `ubuntu`, `migrate`) are mirrored to `ghcr.io/adanalife/mirror/*` (unlimited anonymous pulls, co-located with the runners), every Dockerfile now points at the mirrors, and a weekly `mirror-images` workflow re-copies the tags with crane so they keep tracking upstream security patches. `adanalife/obs-cef-base` stays on Docker Hub — it's our own published image, not a mirror candidate. ([#820])

### fix

- **`!report` validates the Discord webhook URL before POSTing.** A placeholder or garbage `DISCORD_ALERTS_WEBHOOK` no longer logs an "unsupported protocol scheme" ERROR on every report — it warns once per process and falls through to the slog/Sentry audit path. ([#810])

## [v3.1.0] — 2026-06-10

Minor release. The headline is the **VLC command surface completing its move to NATS** — the HTTP command path is gone and NATS is now the sole transport for playback commands, the final step of the migration the observe-only mirror began in v3.0.0. Alongside it: the admin console's chat-send box defaults to the broadcaster identity, and `!ocation` joins the `!location` typo aliases.

### pubsub

- **VLC command surface — HTTP peel; NATS is the sole command transport.** Completes the migration the observe-only mirror started in [#789] (v3.0.0). The client goes publish-only (the `c.get(...)` command calls are gone), vlc-server's subscribers flip from observe-only to acting (driving `PlayRandom` / `PlayVideoFile` / `skip` / `back`), and the `play` / `random` / `skip` / `back` HTTP handlers + routes are removed. The client peel lands first so there's never a window where both transports act. `/vlc/current` stays on HTTP (a read). Requires vlc-server's `NATS_URL` wiring ([infra #645]) to be live in each env. ([#790])

### admin

- **Chat-send box defaults to the broadcaster identity.** The send-from-console form's identity toggle preselected the bot (first in `TokenStatuses`); talking as the channel owner is the common case, so it now preselects the broadcaster when it's logged in, falling back to the bot otherwise. Both stay selectable. ([#807])

### chatbot

- **`!ocation` aliases to `!location`.** A common typo where the viewer drops the leading `l`, added alongside the existing `!location` typo aliases. ([#806])

## [v3.0.1] — 2026-06-09

Patch release. The headline is **sending chat messages from the admin console** — a send box that posts to Twitch chat as the bot or the broadcaster. Alongside it: the OBS image can now be pointed at YouTube as well as Twitch via `STREAM_PLATFORM`, a fix for the OBS websocket-address fallback that broke after the per-platform service rename, and CI changes to cut Docker Hub rate-limiting and stop redundant OBS base rebakes.

### admin

- **Send chat messages from the admin console.** The chat pane gains a send box: type a message and post it to Twitch chat **as the bot** or **as the broadcaster**, with a toggle that only offers accounts currently logged in (and hides itself when just one is). The line renders optimistically (greyed) and confirms when it round-trips back on the live chat stream, reddening as "not delivered" if a send fails. Routed as a `chat.send` NATS command (console publishes, tripbot sends) so it's split-ready. **Broadcaster sends need a re-auth** — `user:write:chat` is new on the broadcaster scopes. ([#803])

### obs

- **OBS streaming target is parametrized via `STREAM_PLATFORM`.** The image hardcoded Twitch in `service.json.tmpl`, so one image could only ever stream to Twitch. A `STREAM_PLATFORM` env var (default `twitch`) now selects the service/server — `twitch` renders byte-identical to the old hardcoded file (a no-op), `youtube` points at YouTube's RTMPS ingest — unblocking a second OBS instance for YouTube without a separate image. ([#775])

### fix

- **OBS websocket address fallback derives from the contract.** The `OBS_WEBSOCKET_ADDR` fallback in `pkg/obs` was hardcoded to the pre-per-platform `obs:4455`; once OBS went per-platform (`obs` → `obs-twitch`) that name stopped resolving, so the watchdog, stream start/stop, and streaming-active poller failed with `dial tcp: lookup obs`. cdk8s never stamps the var onto tripbot, so the default is load-bearing — it now builds from `pkg/contract` (`ServiceOBSTwitch` + `PortOBSWebsocket`) and tracks the canonical service name. ([#804])

### CI

- **Reduce Docker Hub rate-limiting and stop redundant OBS base rebakes.** The tripbot/vlc/obs container CI workflows now authenticate Docker Hub pulls (so base-image pulls count against our account limit instead of the shared GitHub-runner-IP anonymous limit) and gain per-ref concurrency groups so a new push supersedes the in-flight build. The 90-min OBS CEF base bake is scoped to develop/master pushes so release tags no longer rebake an unchanged, version-pinned base. ([#786])

## [v3.0.0] — 2026-06-03

Major release. Two milestones land together. **onscreens-server is now its own image + Deployment** — it is no longer built into or supervised inside the vlc image, which is the breaking deployment change behind the major bump. And the **chatbot no-globals refactor is complete**: the package now holds zero package-level globals, with both the inbound and outbound chat edges behind provider-neutral seams as groundwork for multi-platform chat. The NATS migration also advances — the onscreens command surface is now NATS-only (the HTTP command path is gone) and the vlc command surface begins its observe-only mirror. Rounded out by a live feature-flag toggle and a `!weather` command in the admin/chat surfaces, per-platform service names for the cdk8s app factory, a couple of `!`-command fixes and tweaks, and routine Go + cleanup chores.

### Breaking

- **onscreens-server no longer ships inside the vlc image.** The vlc `Dockerfile` drops the `onscreens-server` build step, its supervisord program, and `:8081` from `EXPOSE`; docker-compose runs `onscreens-server` as its own service. This is the final step of extracting onscreens into a standalone image + Deployment ([#728], shipped earlier) — anything routing to the in-pod copy on `:8081` must be repointed at `onscreens-server:8080` first (prod was repointed via [infra #654]). ([#729])

### pubsub

- **Onscreens commands are NATS-only now.** With the command surface burned in on NATS, the redundant HTTP command path is removed: the client publishes its subject and returns, and `onscreens-server` drops the `middle`/`leaderboard`/`timewarp`/`gps`/`flag` handlers and routes (the overlays are driven by the existing NATS subscribers). The browser-source feeds, health, version, metrics, and admin endpoints stay on HTTP, so `ONSCREENS_SERVER_HOST` is unchanged. With `NATS_URL` unset there's no longer an HTTP fallback — every live env runs NATS. ([#788])
- **VLC command surface — observe-only NATS mirror.** Begins moving the VLC playback commands off direct HTTP onto NATS, following the onscreens template. The four fire-and-forget commands (`PlayRandom`, `PlayFileInPlaylist`, `Skip`, `Back`) now publish to `tripbot.<env>.vlc.<verb>` alongside their HTTP call, and vlc-server connects to NATS and subscribes — but **observe-only**: it logs what it would do without acting, because VLC commands aren't idempotent and acting on both transports would double-execute (skip two videos). HTTP stays the sole actor; this burns in delivery before the peel. `CurrentlyPlaying` is a read and stays HTTP-only. No-op where `NATS_URL` is unset; the peel + cdk8s `NATS_URL` wiring for vlc-server follow. ([#789])

### refactor

- **chatbot no-globals refactor complete — zero package-level globals.** The last global, `client *twitch.Client` (read directly by the package `Say`/`Whisper`), is replaced by a provider-neutral outbound seam: the `IRC` interface becomes `ChatClient` and `App.IRC` becomes `App.Chat`, `twitchChat` implements it over its own client + identity config, and `consoleMirror` wraps any `ChatClient` so every platform's output reaches the admin live console uniformly ([#787]). Preceded by retiring `defaultApp` so tests build their own `App` ([#784]) and collapsing the `sayFn`/`captureSay` test seam onto the injected `recordingIRC` fake ([#785]). The chatbot's inbound (`IncomingMessage`) and outbound (`ChatClient`) edges are now both provider-neutral — adding YouTube/TikTok is a new adapter, not surgery.
- **Admin panel `.controls` disclosure uses the shared CSS variables.** Folds the controls disclosure block into the shared `.stream-preview`/`.now-playing`/`.feature-flags` multi-selector so it reads from the `--divider`/`--dim`/`--muted`/`--fg` variables instead of hardcoded hex, fixing a light-mode inconsistency (no change in dark mode). ([#792])

### feat

- **Live enable/disable toggle for feature flags in the admin panel.** Each feature-flag row gets an arm-then-confirm toggle button backed by a new `feature.FlagToggler` write surface (`SetEnabled`), kept separate from the read-side `FlagClient`. The Postgres-backed client writes the `enabled` column and force-refreshes its in-memory snapshot, so the change is live immediately for both the panel and the bot's command-time gating — no wait for the 30s poll. ([#802])
- **`!weather` — historical conditions at the dashcam location.** New chat command replying with the weather at the currently-playing clip's location *at the time it was filmed* (e.g. `Weather here on Mar 7, 2018 3pm: Clear sky, 58°F`), sourced from the free Open-Meteo historical archive (reanalysis back to 1940, so it covers the 2018 corpus). Follows the App-injection pattern (`App.Weather` + `realWeather`/`noopWeather`, mirroring `App.Geocoder`) and is follow-gated like `!sunset`/`!location`. ([#799])
- **Per-platform service names in the contract.** Adds `tripbot`/`vlc`/`onscreens` per-platform service-name constants (`tripbot-twitch`, `vlc-youtube`, …), mirroring obs, as the source-of-truth half of the unified per-platform cdk8s app factory. The bare app-identity keys stay for Secret/ConfigMap names. ([#798])
- **`!miles` floors displayed monthly miles at 0.01.** A user with a tiny but non-zero monthly total no longer renders as `0`. ([#796])
- **`!leaderborad` aliases to `!leaderboard`.** Common typo now resolves instead of missing. ([#794])

### fix

- **`!location` suppresses the `0,0` fallback Maps URL.** When coordinates are unavailable, the command no longer emits a link pointing at null island. ([#795])
- **Admin OBS nav link derives from `OBS_SERVER_HOST`.** The admin panel's OBS link is built from the configured host value instead of a hardcoded assumption. ([#793])

### chore

- **Go 1.26.3 → 1.26.4** for stdlib CVE fixes. ([#797])
- **Removed the stale `Dashcam_Scenes.linux.json`** OBS scene collection. ([#791])

## [v2.18.3] — 2026-06-02

Patch release. The headline is reboot-survival for the admin live console: its chat log and live map are now backed by JetStream, so a tripbot restart replays recent history instead of starting empty. The rest is the chatbot no-globals refactor reaching its conclusion — the `SetX` injection setters retire in favour of cmd assigning the App's dependencies directly, the package free-function shims are gone, and a `New()` constructor plus a platform-neutral inbound seam land as groundwork for multi-platform chat.

### pubsub

- **Live-console chat log + live map survive a reboot.** Both were in-memory ring buffers fed by core NATS, which has no replay — so every restart started the console empty until new messages arrived. They now bind ephemeral ordered JetStream consumers over two bounded file-backed streams (`TRIPBOT_CHAT` keeps 500, `TRIPBOT_VIDEO` keeps 200) and replay recent history into the buffers on startup. Publishers are unchanged; only the consumer side moved. Falls back to live-only core subscriptions when JetStream is unavailable (local dev, non-JetStream servers), so nothing breaks without it. Requires [infra #623] for file-backed JetStream + a PVC on the NATS deployment. ([#744])

### refactor

- **chatbot — retire the `SetX` setters; cmd owns the App.** The remaining package-level injection setters are removed in favour of `cmd/tripbot` assigning the App's dependencies directly: `App.Cron` ([#779]), `realVideo`'s own `Player` ([#780]), `App.Sessions` + `App.UserSessions` ([#781]), and `App.Flags` ([#782]). The package free-function shims retire with cmd taking ownership of the App ([#774]), preceded by a `New()` constructor + `ConnectIRC` method ([#773]) and a platform-neutral inbound seam (`IncomingMessage` + `Handle*` methods) that isolates the Twitch-specific entry points behind a transport-neutral interface ([#772]).

## [v2.18.2] — 2026-06-02

Patch release, almost entirely internal. The no-globals refactor advances on every front: the last per-package `defaultX` singletons (`pkg/server`, `pkg/video`, `pkg/users`) are retired in favour of constructed structs threaded from cmd, cmd's own globals move into a `Tripbot` struct, and the chatbot's command registry, dispatch path, and event handlers move onto the injectable `App`. The NATS migration moves the onscreens command surface (and the admin panel's now-playing) onto pub/sub. Rounded out by a logout-path crash-loop fix, the producer half of the tripbot↔infra anti-drift contract, a `go test` env-default fix, and a CI action bump.

### fix

- **Logged-out users no longer crash-loop `save()`.** A user whose DB row couldn't be found or created (a transient `Find` error returns `ID: 0`) was cached in the session and then failed GORM's `Updates()` on every logout tick with "WHERE conditions required". `save()` now skips a zero-ID user and `login()` won't cache one, so the session self-heals on the next tick. No data lost — the `events` log and monthly scoreboard were unaffected; only the `users.miles` cache missed its increment, and it is recomputable. (TRIPBOT-8D, [#778])

### pubsub

- **Onscreens command surface on pub/sub.** The onscreens overlay commands move from direct HTTP calls onto NATS, extending the pub/sub substrate. ([#736])

### refactor

- **Retire the last package `defaultX` singletons.** `pkg/users` session state is encapsulated behind a constructed `*Sessions` with an injected `ChatterSource` ([#753]) and then has its `defaultSessions` global retired ([#764]); `pkg/server` retires `defaultServer`, threading a `*Server` through cmd ([#757]); `pkg/video` retires `defaultPlayer` and sources the admin panel's now-playing from NATS ([#758]).
- **cmd globals lifted into a `Tripbot` struct.** The `cmd/tripbot` entrypoint constructs and threads its dependencies instead of reaching for package globals. ([#755])
- **chatbot registry, dispatch, and handlers onto the `App`.** Social-media replies ([#766]), the dispatch path with access-check denials ([#768]), follower/subscriber announcements and the Chatter timer ([#769]), and the command registry with `findCommand` ([#770]) all move off package-level globals onto the injectable `App`, routing chat output through `a.IRC.Say`. Groundwork for retiring `defaultApp` and the `sayFn` global, and for multi-platform chat support.

### CI

- **`go test` defaults `ENV` to testing.** With `ENV` unset under `go test`, `config.SetEnvironment` defaulted to `development` and failed on the absent `.env.development`; it now defaults to `testing` via `testing.Testing()`, so bare `go test ./pkg/...` loads the checked-in `.env.testing` with no prefix — completing the repo-root resolution from v2.18.1. ([#767])

### tooling

- **tripbot↔infra anti-drift contract (producer half).** `pkg/contract/` holds the canonical service names, ports, and env-var keys as typed Go constants (cross-checked against `pkg/config/tripbot`, `pkg/obs`, `pkg/database`); a `go:generate` tool emits `contract.json` and a test fails on drift in either direction. A new `contract.yml` workflow regenerates in CI and fails if the committed file is stale. The infra cdk8s manifests consume it via `task contract:sync`. Dependency-free, so generate + test stay fast and hermetic. ([#777])

### deps

- **`jdx/mise-action` 2 → 4.** ([#759])

## [v2.18.1] — 2026-06-01

Patch release. Mostly internals: the no-globals refactor lands its final package conversions — `pkg/twitch` and `pkg/server` now construct a `*API` / `*Server` instead of mutating package-level globals — alongside extracting reverse-geocoding into an injectable `pkg/geo` and giving chatbot a `App.Twitch` injection seam. Admin-panel polish continues (htmx live updates replacing full-page reloads, a GPS-jump-aware map trail, and an offline-collapsed stream preview), plus dashcam-cv database groundwork (pgvector), a `go test` ergonomics fix, and an OpenTelemetry deps bump.

### admin panel

- **htmx live updates replace full-page reloads.** Restart and stream-toggle buttons `hx-post` and swap their widget in place instead of full-page POST + redirect (which reloaded the Twitch preview, re-initialized Leaflet, and lost chat scroll). Adds in-flight feedback (`hx-disabled-elt` + a dim/progress-cursor style) and a hidden 15s poller that OOB-swaps the service-status rows and stream toggle so they stay current without a reload. ([#752])
- **Map trail breaks on GPS jumps.** The live map split the breadcrumb trail into solid runs of consecutive points within 50 km and renders each cross-jump gap as a faint dashed bridge, so a timewarp clip or bad GPS fix no longer slashes a straight line across the map. ([#749])
- **Stream preview collapses when offline + shorter service labels.** The preview disclosure now defaults open only when OBS reports an active stream (was hardcoded open, always loading the Twitch player), and the status rows read `vlc` / `onscreens` instead of `vlc-server` / `onscreens-server`. ([#735])

### dashcam-cv

- **pgvector `frame_embeddings` table + `cv:stats` task.** Migration 015 adds a `vector(1152)` embeddings column (SigLIP2 so400m NaFlex) with HNSW cosine + unique indexes and `CREATE EXTENSION vector`; local dev Postgres moves to the `pgvector/pgvector:pg16` image. Migration 016 drops the dead `moments`/`viewings` tables. Adds a `cv:stats` task for coverage/size/rate via psql. ([#750])

### refactor

- **`pkg/twitch` → `*API`.** The package's mutable globals are encapsulated in a constructed `*API` with `New()`; existing exported functions keep thin shims delegating to a `defaultClient`, so external callers are unchanged. Marks the auth-core seam for the eventual standalone Helix service. ([#738])
- **`pkg/server` → `*Server`.** The last of the package conversions: `eventHub`, `twitchConnected`, `versionTag`, and the feature-flag client move onto a constructed `*Server` (package-level shims back a `defaultServer` singleton, so cmd/tripbot is unchanged). Deletes a dead `var server`. ([#754])
- **reverse-geocoding extracted into `pkg/geo`.** `helpers.CityFromCoords` / `StateFromCoords` no longer reach into the geocoder SDK's package global from a pure utility package. New `pkg/geo` holds a `Geocoder` interface + `*Client` (API key as a field); `helpers` goes back to dependency-free. ([#747])
- **chatbot: inject Twitch Helix surface as `App.Twitch`.** `followageCmd` calls `a.Twitch.FollowedAt(...)` through an injected interface instead of the `pkg/twitch` package global, continuing the chatbot-app-injection pattern and unlocking unit tests for the command. ([#751])

### CI

- **`go test` finds the repo-root `.env.testing` from any directory.** `config.SetEnvironment` resolved the dotenv file with a cwd-relative `godotenv.Load`, so a package's test binary — which runs from its own dir — never found the checked-in `.env.testing` and either `log.Fatalf`'d in a config `init()` or required a manual `set -a; . ./.env.testing; set +a`. The lookup now anchors at the module root (nearest ancestor with `go.mod`), so `ENV=testing go test ./pkg/...` works with no sourcing, matching the `task test` / `task test:macos` paths. Deployed binaries with no `go.mod` ancestor fall back to the bare relative path, preserving cluster behavior. ([#743])

### deps

- **OpenTelemetry instrumentation bumps** — `contrib/instrumentation/net/http/otelhttp` and `contrib/instrumentation/runtime` 0.68.0 → 0.69.0 (with `otel/sdk` 1.43 → 1.44 to match core). ([#746])

## [v2.18.0] — 2026-05-29

Minor release. The admin panel's live console fills in around the chat pane shipped in v2.17.2: a live viewer count, a now-playing card that updates the instant the video changes, a token-expiry countdown, usable chat scrollback that only auto-scrolls when pinned to the bottom, a click-a-username profile popover (monthly miles + handling for 'unknown' dates), and a live location map with a breadcrumb trail and a 'show full route' corpus overlay. onscreens-server moves to its own standalone slim image with a multi-arch release pipeline. Internals: background jobs construct a `*Scheduler` instead of reaching for a package global, and the OpenTelemetry dependencies get bumped.

### admin panel

- **Live viewer count.** Current-viewer tally on the panel. ([#725])
- **Now-playing card live-updates on video change.** The card refreshes when the playing clip changes instead of waiting for a page reload. ([#726])
- **Live token-expiry countdown.** Shows time remaining on the Twitch token, driven by the `tripbot_twitch_token_expires_at_seconds` gauge added in v2.17.0. ([#727])
- **Usable chat scrollback.** The live chat pane only auto-scrolls when you're pinned to the bottom, so scrolling up to read stays put. ([#730])
- **Chat user-profile popover.** Click a username in the chat console to open a profile popover ([#731]); it shows the user's monthly miles and handles 'unknown' dates gracefully ([#732]).
- **Live location map.** A map on the panel with a breadcrumb trail of recent positions ([#733]), plus a 'show full route' overlay that draws the whole corpus route ([#734]).

### onscreens

- **Standalone slim image + multi-arch release pipeline.** onscreens-server builds as its own slim image with a dedicated multi-arch (amd64 + arm64) release pipeline, rather than riding along in another image. ([#728])

### refactor

- **background: construct a `*Scheduler` instead of a package global.** Background jobs are wired through a constructed `*Scheduler`, removing the package-level global. ([#737])

### deps

- **OpenTelemetry bumps** — `otel/log` 0.20.0 ([#706]) and `contrib/bridges/otelslog` 0.19.0 ([#705]).

## [v2.17.2] — 2026-05-29

Patch release. The admin panel gains a **live chat console** — recent chat history renders on load and new messages stream in real time over Server-Sent Events, fed by a new NATS observation-event bus (`pkg/eventbus`). It shows the bot's own output (Twitch doesn't echo sent messages back), per-message timestamps localized to the viewer, and a stable color per username. Shipped as a vertical slice (chat only) on the SSE+htmx foundation; live now-playing / viewer count / reauth cards are follow-ups.

### admin panel

- **Live chat console (SSE + NATS).** New `pkg/eventbus` publishes fire-and-forget *observation* events (`tripbot.<env>.chat.message`, snake_case JSON + `emitted_at`) over the `pkg/natsclient` singleton — distinct from `pkg/events` (the Postgres session log) and from onscreens *commands*. A `pkg/server` hub subscribes, keeps a 200-line in-memory recent-history ring, and fans out to browser clients on `GET /admin/events` with non-blocking sends (a slow client drops events, never stalls the NATS callback). The panel renders recent history server-side and streams new lines via htmx's SSE extension (htmx 2.x + the SSE ext vendored + embedded so the binary stays self-contained). The hub starts after `startNATS()` and sources only from NATS, leaving the panel splittable into its own service later. See the admin-live-console decision. ([#719])
- **Bot output in the console.** `chatbot.Say()` mirrors the bot's own output onto the event bus, and `realIRC` now delegates to the package `Say`/`Whisper` so command responses (which go through `a.IRC.Say`) flow through the same single emit path instead of a duplicated body that skipped it. ([#722], [#723])
- **Per-message timestamps + per-user colors.** Each line shows its time (UTC rendered server-side as a fallback, localized to the viewer's timezone in JS) and a stable hue derived from a hash of the username — same chatter, same color every time, tuned to stay legible on both panel themes. ([#722])

### fixes

- **SSE stream no longer recycles every ~20s.** The HTTP server's 15s `WriteTimeout` severed the long-lived `/admin/events` response — and `http.ResponseController.SetWriteDeadline` returns "feature not supported" through the negroni + otelhttp (httpsnoop) HTTP/2 wrapper chain — so live chat only appeared on reload. Set `WriteTimeout: 0` + `ReadHeaderTimeout: 15s` (the real slowloris guard). ([#720])

### ci

- Vendored frontend assets (`pkg/server/static/`) excluded from the linters — pre-commit + super-linter path excludes, a `biome.json` ignore, and the redundant ESLint/Prettier-JS linters disabled (Biome covers JS; the only JS in the repo is the vendored htmx/sse). ([#719])

## [v2.17.1] — 2026-05-28

Patch release. Hotfix for the v2.17.0 silent-disconnect watchdog's import chain — it dragged `pkg/config/tripbot` and `pkg/database` into vlc-server's binary at link time, so the vlc-server pod refused to boot in prod without 9 placeholder env vars stamped into its ConfigMap. Took prod's dashcam playback down for 13 minutes during the rollout. Detangled by moving the watchdog into its own subpackage; companion fix broadens the `release-development.yml` vlc paths-filter so shared-package edits like this one don't silently skip the vlc rebuild.

### obs

- **Move the silent-disconnect watchdog out of `pkg/obs` into `pkg/obs/watchdog`.** The watchdog file imported `pkg/config/tripbot` (for `ChannelName`) and `pkg/twitch` (for `IsChannelLive`) at the package level — and `pkg/twitch` transitively imports `pkg/oauthtokens` → `pkg/database`, whose `init()` `log.Fatalf`s on missing `DATABASE_USER` / `DATABASE_DB` / `DATABASE_HOST`. Because all files in a Go package compile together, anything importing `pkg/obs` inherited the whole chain. `cmd/vlc-server/vlc-server.go` has imported `pkg/obs` for ages for one call (`obs.PollStreamingActive`), so v2.17.0's vlc-server binary refused to boot without env vars for the 6 required `pkg/config/tripbot` keys + the 3 `pkg/database` checks — even though the watchdog never runs inside vlc-server. After the move, `pkg/obs` holds only `control.go` + `streaming.go` (zero tripbot-internal imports beyond `pkg/instrumentation`), and `cmd/tripbot` is the sole consumer of the new `pkg/obs/watchdog` subpackage. Verified: `go list -deps ./cmd/vlc-server` no longer includes `pkg/config/tripbot`, `pkg/database`, `pkg/twitch`, or `pkg/oauthtokens`. Same for `./cmd/onscreens-server`. ([#716])

### ci

- **Broaden the `release-development.yml` vlc paths-filter to `pkg/**`.** The narrow vlc filter (only `pkg/{vlc,onscreens}-{server,client}/`) meant any change to a shared package — `pkg/obs`, `pkg/instrumentation`, `pkg/telemetry`, etc. — silently skipped the vlc rebuild. `#716`'s detangle would have shipped a stale vlc `:develop` image on the first run; stage would have continued running the bug. The new filter matches the tripbot filter's shape — broad enough to never miss a transitive-dep change, at the cost of some unnecessary rebuilds (the native-runner vlc build is ~3-4 min, cheap insurance). ([#717])

## [v2.17.0] — 2026-05-28

Minor release. TripBot gets a live Discord bot session (four slash commands mirroring the Twitch leaderboards), gated behind the first flag of a new Postgres-backed feature-flag system with a read-only admin-panel listing. NATS adoption begins — `ShowMiddleText` parallel-publishes to `tripbot.<env>.onscreens.middle.show` while HTTP stays the source of truth. A silent-disconnect watchdog auto-recovers from the prod failure mode where OBS's RTMP write socket goes half-open and frames keep streaming into the void while Twitch shows offline. Twitch token handling closes a cron-desync gap that bit prod on 2026-05-26 (refresh-on-startup + expiry-timestamp gauge, plus a 30→45 minute refresh window that spares the helix 401 self-heal from firing on the routine cycle). Leaderboard rendering swaps space-padded monospace for CSS grid alignment so scores line up under the regular Trebuchet stack.

### feature flags

- **Postgres-backed feature flag substrate.** New `pkg/feature` package exposes `FlagClient.Bool(ctx, key, evalCtx) bool` (OpenFeature-shaped — swap is one file when a second provider or typed variants arrive). `PostgresClient` refreshes a 30s in-memory cache in the background and retains the last-known-good snapshot on DB failure, so a network blip can't flip features. Boolean flags only for v1; targeting is per-username allowlist > per-role allowlist > global default. `target_removal_date NOT NULL` on the schema means every flag is born with a tombstone. Unknown keys evaluate to false as a typo-safety property. Chatbot's `App` gains `Flags feature.FlagClient`, wired to `realFlags{}` in `defaultApp` (per the chatbot-app-injection-pattern ADR). `cmd/tripbot.startFeatureFlags` builds the Postgres client between `startCron` and `chatbot.Initialize`; non-fatal on startup failure. ([#710])
- **Read-only feature flags section on the admin panel.** Disclosure under "now playing" listing every flag the `FlagClient` knows about — key (monospaced), description, and on/off dot. Hidden during the startup window between the HTTP server starting and `startFeatureFlags` swapping in the Postgres-backed client. Reads `FlagClient.Snapshot(ctx)`, a new interface method on both the in-memory and Postgres clients; `Flag` grows `Description` + `TargetRemovalDate` (date held back from the render so the panel stays phone-sized). Foundation for the planned `/admin/flags` CRUD endpoints. ([#713])

### discord

- **TripBot gets a live Discord bot session — first pass.** New `pkg/discord` opens a `bwmarrin/discordgo` gateway and registers four guild-scoped slash commands in ADanaLife: `/leaderboard` (monthly miles), `/totalleaderboard` (lifetime miles), `/guessleaderboard` (correct guesses this month), and a static ephemeral `/commands` listing the others. Names mirror the existing Twitch `!leaderboard` / `!totalleaderboard` / `!guessleaderboard` triggers so muscle memory transfers. Two new optional config fields (`DISCORD_BOT_TOKEN`, `DISCORD_GUILD_ID`); the bot stays disabled in any env where either is unset, empty, or still at the AWS Secrets Manager placeholder string — `pkg/discord.ShouldStart` is the single decision point that skips startup cleanly with one INFO log line. Every other failure path (auth failure, command-registration failure, gateway drop) is fail-open: tripbot's core IRC / EventSub paths are never blocked or crashed by Discord. Companion infra lands in `infra` (terraform SM containers + ESO + Deployment `envFrom`). Deferred to follow-up passes: OAuth Discord↔Twitch linking, `/miles <user>`, stream-state commands, retiring the existing `DISCORD_ALERTS_WEBHOOK`. ([#700])
- **Gate Discord startup behind the `discord.bot_enabled` feature flag.** First production use of the flag system landed in #710. `startDiscord` evaluates the flag after the config-shaped `ShouldStart` check and returns early when it's false. Defaults off — both envs are gated until a row exists with `enabled=true`, so prod stays Discord-less and stage flips on with a one-shot `UPDATE feature_flags SET enabled=true WHERE key='discord.bot_enabled'`. Migration 014 seeds the flag with `enabled=false` and a `target_removal_date` 6 months out so it shows up in the admin panel from day one. ([#712])
- **Diagnostic gateway logging.** `handleInteraction` logs every interaction it receives (type + guild_id + channel_id) before any type filter, and `discordgo.Session.LogLevel` flips to `LogInformational` so the library's HELLO / READY / dispatched-event chatter surfaces in our logs. Added to chase a stage symptom where `/leaderboard` autocompleted and Discord showed "is thinking…" but no `INTERACTION_CREATE` event ever surfaced in tripbot logs. ([#707])
- **Stop deleting commands on shutdown + measure `/commands` reply latency.** Found via a stage rollout-restart sequence: commands vanished from Discord client autocomplete even though both old and new pod logs showed successful registration. The race was on shared command IDs — `ApplicationCommandBulkOverwrite` from the new pod updates entries in place, then the old pod's `Stop()` deletes the IDs it had stored (which are now the new pod's IDs too). Fix: `Stop()` only closes the gateway; `BulkOverwrite` is the right reconciliation primitive across pod cycles. Companion change times `InteractionRespond` and logs the outcome so a silently-slow reply (exceeding Discord's 3s response window but eventually returning nil) becomes visible. ([#708])

### nats

- **`ShowMiddleText` parallel-publishes to `tripbot.<env>.onscreens.middle.show`.** Migrate one fire-and-forget HTTP call as proof of the pattern. HTTP remains the source of truth — a NATS outage / misconfig is invisible to viewers, and the on-screen overlay always lands via at least one path. New `pkg/natsclient` package holds a process-wide `*nats.Conn` singleton (mirroring `pkg/database`'s `SetGormDB` shape); empty `NATS_URL` → conn stays nil, all publishes no-op silently. Chatbot's `App` gains `App.NATS` (one method, `Publish`) + a `realNATS` adapter; `realOnscreens` carries the NATS conn + env and mirrors each `ShowMiddleText` call. Subscriber side: `onscreens-server.Server.StartNATSSubscribers` JSON-decodes the envelope and calls the same `s.MiddleText.Show(msg)` path the HTTP handler takes. Wire format is snake_case JSON (`{ "msg": "...", "emitted_at": "..." }`) so an eventual protobuf swap is a 1-1 schema mapping. The HTTP-fallback peel and JetStream durability come later. ([#711])

### obs

- **Silent-disconnect watchdog.** New `pkg/obs/silent_disconnect_watchdog.go` polls OBS `GetStreamStatus.outputActive` against Helix `GetStreams` and force-restarts the OBS stream after N consecutive misalignments. Addresses the prod failure mode where Twitch's ingest closes the session without the FIN/RST making it back to OBS (idle middlebox, or Twitch-side termination), leaving the write socket half-open: OBS reports `outputActive: true / outputCongestion: 0.0 / outputReconnecting: false`, frames keep getting written into the void, Twitch shows offline. Hit prod 2026-05-27 ~30h into a session; manual recovery was `StopStream` + `StartStream` via OBS WebSocket — the exact sequence this automates. Defaults: 60s poll, 3-miss debounce (3 min), 10 min cooldown between forced restarts. Hooks are injectable for unit-testability; failure modes fail safe (OBS unreachable / Helix transient error → reset misses, skip; OBS not active → no-op). New metrics: `tripbot_twitch_channel_live` gauge + `tripbot_obs_silent_disconnect_restarts_total` counter, read by paired Grafana alert rules in [adanalife/infra#611](https://github.com/adanalife/infra/pull/611/changes). ([#702])
- **Watchdog skips misses when OBS already knows it's reconnecting.** New `GetStreamActiveSteady` (`outputActive=true AND outputReconnecting=false`) replaces the simpler check in `DefaultWatchdogDeps.OBSActive`. Stage synthetic (Cilium NetworkPolicy blocking RTMP egress) surfaced a false-positive surface: OBS entered its built-in reconnect loop within ~16s but the watchdog was already counting misses against it — would have force-restarted at miss 3 even though OBS was actively recovering. The truly silent half-open from the 2026-05-27 prod incident had `outputReconnecting=false`, which is the only state the watchdog exists to handle. Admin panel keeps the simpler `GetStreamStatus`. ([#709])

### twitch

- **Refresh tokens on startup.** Closes a cron-desync gap where a restart within ~30min of a token's expiry leaves the bot stale until the hourly cron catches up. Bit prod on 2026-05-26: pod restarted at 18:35 (~27min before the bot token's 19:02 expiry), `gocron`'s `DurationJob(1*time.Hour)` doesn't fire until exactly one interval after `Scheduler.Start()` (19:35), and the admin-panel "expired" banner appeared at 19:02 — 33 minutes of staleness despite #636's helix-401 self-heal (which couldn't catch it because `GetChannelChatChatters` was still succeeding through Twitch's grace window). `cmd/tripbot` now calls `mytwitch.RefreshUserAccessToken(ctx)` synchronously after `loadTwitchToken` and before `setUpTwitchClient`; `refreshOne` early-returns when the stored token is healthy, so this is a no-op in the common case. ([#698])
- **`tripbot_twitch_token_expires_at_seconds{account}` gauge.** Unix-timestamp expiry per identity, recorded from every place that writes a token slot — `LoadFromDB` (both success and broadcaster-missing paths), `applyBotToken`, `applyBroadcasterToken`. A blanked token writes 0 naturally. Drives the forthcoming "tripbot needs reauth" alert (filed separately in infra) and unlocks the "time remaining in session" admin-panel item. The existing `tripbot_twitch_connected == 0 for 5m` alert can't catch user-access-token expiry: IRC stays connected when the token expires because Twitch only checks at PASS time on initial connect. ([#698])
- **Widen the refresh window from 30m to 45m.** Bumps `time.Until(t.ExpiresAt) > 30*time.Minute` to `> 45*time.Minute` in `refreshOne`. The hourly cron at 30 min was missing Twitch's variable-early enforcement, producing one self-healed 401 per ~4h token cycle per identity (≈12/day across bot + broadcaster) — recovery is sub-second via #636, but the ERROR-level slog line ships to Sentry and one event per identity per 4h cycle looks alarming when it's actually proof the self-heal is working. The math: hourly cron with a 30-min window means exactly one fire per cycle lands in `[T+3h30m, T+4h]`; Twitch enforces around `T+3h50m` (variable), so a cron firing at `T+3h35m` sees 25m left and skips — and the next fire is past Twitch's enforcement. 45m raises the floor: any cron fire in `[T+3h15m, T+4h]` refreshes proactively. ([#699])

### admin panel

- **Stream preview defaults to expanded.** The Twitch player disclosure now opens on page load instead of needing a click. Iframe wiring keeps the existing collapse-to-pause behaviour (collapse swaps `src` to `about:blank` so audio + bandwidth stop), and re-expanding restores the player from `data-src`. Initial render bootstraps `src` directly when the disclosure starts open since the `toggle` event only fires on user interaction. ([#714])

### leaderboard

- **Align scores via CSS grid, drop the monospace requirement.** [#689](https://github.com/adanalife/tripbot/pull/689/changes) left scores `%-*s`-padded and forced the leaderboard onscreen to a monospace stack so the spaces would render as a column. In practice the monospace swap didn't take post-launch. `pkg/users.LeaderboardContent` now emits `<div class="lb-grid"><span class="lb-score">…</span><span class="lb-user">(user)</span>…</div>` instead of space-padded plaintext. Each onscreen registers a new `RenderAsHTML bool` in `onscreenRegistry` — the leaderboard sets it; the rest stay on the legacy `textContent` path. `templates/onscreen.html.tmpl` carries the matching CSS (`display: inline-grid`, `grid-template-columns: auto auto`) and reads `data-html` off `#root` to pick `innerHTML` vs `textContent`. Font reverts from `"Menlo", "Consolas", "Courier New"` → `"Trebuchet MS", sans-serif` to match the other onscreens. Defensive `html.EscapeString` of title / score / user. The grid auto-sizes the score column to its widest entry, so digits left-align across rows in any font. ([#701])

## [v2.16.1] — 2026-05-26

Patch release. First post-launch cut. The inter-clip gap now hides behind a still frame of the next clip's opening rather than the broken-video overlay (still + fix). New `release-development` CI workflow publishes `:develop` multi-arch images on every push to develop, so stage-1 can ride develop without manual rebuilds. Admin panel learns to colour its env badge and put the env in `<title>` so prod and stage tabs stop looking identical. Chatbot's social roster modernises (`!bluesky` + `!tiktok` added, `!socialmedia` listing follows). Scoreboards filter zero-score rows and align in a monospace column; the `/onscreens/state.json` access-log flood gets demoted to debug.

### streaming

- **Cover the inter-clip gap with the next clip's first frame.** vlc-server's poll goroutine (5s) shells out to `ffmpeg` whenever `currentlyPlaying()` changes and stashes a single-slot JPEG at `RunDir/next-frame.jpg`, served at `/next-frame.jpg` plus a self-polling HTML wrapper at `/next-frame.html`. OBS's Main scene gains a `Next-frame preview` browser source between `Broken video warning` and `Dashcam`, pointed at `vlc-server`'s `/next-frame.html` via the new `VLC_URL_BASE` envsubst (mirrors `ONSCREENS_URL_BASE`). Visible during the inter-clip gap when `Dashcam` clears, hidden under the dashcam video the rest of the time. The HTML wrapper refreshes the `<img>` every 10s with a cache-bust query so CEF tracks file changes without an obs-websocket push; the JPEG response carries `Last-Modified` so polls 304 when unchanged. `ffmpeg` joins the vlc container's apt list; no new Go deps. ([#695])
- **Tell ffmpeg the output muxer for the first-frame extract.** Stage logs showed every extraction failing with `Error initializing the muxer for /opt/data/run/next-frame.jpg.tmp: Invalid argument` because `ffmpeg` infers the muxer from the file extension and `.tmp` isn't a known image format. Adds `-f image2 -update 1` to make the muxer + single-image semantics explicit, so the atomic-write tmp extension stops mattering. ([#696])

### ci

- **`release-development.yml` publishes `:develop` images on push to develop.** Tag-publish workflow mirroring `release.yml`'s matrix-on-native-runners + manifest-list shape per the multi-arch-release-pattern ADR. Per-image `dorny/paths-filter` gates the three builds — tripbot rebuilds on `cmd/tripbot/`, `pkg/`, `infra/docker/tripbot/`, `go.{mod,sum}`; vlc on `cmd/{vlc,onscreens}-server/`, `pkg/{vlc,onscreens}-{server,client}/`, `infra/docker/vlc/`, `go.{mod,sum}`; obs on `infra/docker/obs/`. The OBS filter is the load-bearing one (arm64 leg's CEF compile is ~30 min, so skipping it on Go-only merges is real CI savings). Tag scheme: per-arch `:develop-{amd64,arm64}` + manifest list `:develop`. No `latest=` flag — prod's `:latest` stays release-tag-driven. `VERSION` build-arg stamps as `develop-<short-sha>` so `/version` endpoints on stage identify the running commit. `verify-stamped-image.sh` runs per-arch as the corruption check, same as `release.yml`. ([#694])
- **`.dockerignore` comment.** Documents the broad-ignore + re-include pattern (excludes `infra/`, re-adds `infra/docker/{obs,vlc/config,bin}`) so the next person who touches it knows the convention. An earlier rewrite to explicit subdir-ignores was considered and rolled back — the broad-ignore stays. ([#691])

### admin panel

- **Render env in `<title>` and colour the env badge.** When running prod + stage admin tabs side-by-side, the page titles were identical and easy to mix up. `<title>` now reads `tripbot — <channel> (<env>)`, and the env chip in the header takes on a colour per env: green for prod, yellow for stage, blue for dev, neutral for testing / unknown. Both themes get tuned chip colours so the badge stays legible in light + dark. `c.Conf.Environment` is the source of truth — same field OTLP's `deployment.environment` reads. ([#687])

### chatbot

- **`!bluesky` (+ `!bsky` alias) and `!tiktok` commands.** Both mirror the existing per-platform commands (`!twitter` / `!instagram` / `!facebook` / `!youtube`). `!tiktok` handle is bare `adanalife` per the branding ADR; `!bluesky` uses the custom domain `dana.lol`, with `!bsky` as an alias since that's the most common shortened form. ([#690])
- **`!socialmedia` listing follows.** Twitter demoted in favour of Bluesky; Facebook drops from the summary. The `!twitter` and `!facebook` commands stay registered so anyone who types them still gets a response — they're just no longer in the digest. ([#690])

### scoreboards

- **Filter zero-scorers from the monthly guess leaderboard.** `AddToScoreByName` uses `FirstOrCreate`, so every user who's ever attempted a guess gets a row at `value=0` — noisy early in each month. Rows whose integer score parses as `0` or empty are filtered before render; if every row is filtered, the overlay skips entirely and the chat command falls back to its existing "No one is on that leaderboard yet!" message. ([#689])
- **Left-align the score column in the leaderboard onscreen.** Mixed 1/2/3-digit guess counts (or 4/5/6-char miles values) jumped left/right between rows because the score field had no padding. `LeaderboardContent` now pads via `%-*s` to the longest score's width, and the leaderboard onscreen's font switches from `Trebuchet MS` to a monospace stack (`Menlo` / `Consolas` / `Courier New`) so space-widths align with digit-widths. Shared path, so monthly miles + lifetime miles leaderboards benefit too. ([#689])

### obs

- **Flatten the `MIDDLE` group so middle-text renders in bottom-center.** The `Middle Text Onscreen - Center` browser source was wrapped in a single-item `MIDDLE` group at canvas `(0, 0)` with the source positioned at `(640, 1033)` inside the group — but in the live scene that resolved to top-left rather than between the rotators (the group layer was swallowing the in-group position). The group did nothing useful (one child, no transforms), so it goes and the source promotes to a top-level scene item at the same `(640, 1033)`. Lands between the left rotator (x=0..640) and the right rotator (x=1280..1920) on the bottom strip where it belongs. ([#692])
- **Lower default SomaFM volume to -18.3 dB.** Groove Salad was a touch too loud at full volume on first sound-check. Live-tweaked on launch night via the noVNC mixer to `-18.3 dB` (0.121619 mul) — peaks barely clipping into yellow, in the verification-runbook target zone of ~-20 to -10 dB. Bake the same value into the seed scene config so it survives pod restarts and image rebuilds. ([#693])

### telemetry

- **Demote `/onscreens/state.json` access log to debug.** OBS's CEF browser sources poll the endpoint at ~14 req/sec idle, flooding the default INFO stream. Extends the existing health-probe debug-path allowlist (#583) to cover it; renames `healthPaths` → `debugPaths` to reflect the broader purpose. ([#688])

[#687]: https://github.com/adanalife/tripbot/pull/687
[#688]: https://github.com/adanalife/tripbot/pull/688
[#689]: https://github.com/adanalife/tripbot/pull/689
[#690]: https://github.com/adanalife/tripbot/pull/690
[#691]: https://github.com/adanalife/tripbot/pull/691
[#692]: https://github.com/adanalife/tripbot/pull/692
[#693]: https://github.com/adanalife/tripbot/pull/693
[#694]: https://github.com/adanalife/tripbot/pull/694
[#695]: https://github.com/adanalife/tripbot/pull/695
[#696]: https://github.com/adanalife/tripbot/pull/696

## [v2.16.0] — 2026-05-25

Minor release. The launch-day cut. Follower-gating goes off (kill-switch, default off) so any viewer can drive the bot during launch + soak. The prod admin-panel stream-preview hack from v2.15.5 reverts now that the cutover is in flight. New `!makebot` / `!unbot` admin commands on top of a deeper users-package cleanup — the hardcoded 107-entry ignore-list retires in favor of the persisted `is_bot` column as the single source of truth, and `TopUsers` picks up the bot filter it was missing. Telemetry pass drops three sources of recurring log/Sentry noise. Admin panel learns to route dashboard links over the tailnet so they stop dead-ending on LAN-conflicting client networks. CI gains govulncheck; tests grow Go fuzz targets that caught two parser bugs in their first minute.

### chatbot

- **Disable follower-gating for launch.** New `followerGatingEnabled` kill switch (default `false`) wraps the `RequiresFollow` check in `checkAccess`, so launch-day viewers can run any command without being told to follow first. The 17 `RequiresFollow:true` registry entries stay as-is — flip the var back to `true` to re-enable when soak is over. ([#679])
- **`!makebot` / `!unbot` admin commands.** Manual curation surface for `users.is_bot`. Both are admin-only and silent in chat — outcome logged at slog Info for ops visibility. Usage: `!makebot <username>` / `!unbot <username>`. Leading `@` is stripped and the target is lowercased before lookup; unknown targets log a warn and return without panic. Goes through the new `App.Sessions.SetBot` seam with `recordingSessions` covering the path end-to-end. ([#675])

### users

- **Drop the hardcoded `IgnoredUsers` list; `is_bot` is the single source of truth.** The 107-entry slice in `pkg/config/tripbot/helpers.go` was a backstop for the leaderboard query — went stale, masked drift in `users.is_bot`, and required a PR to flag new bots. With the DB dump already correctly annotated the column alone is sufficient; channel owner is excluded separately via `UserIsAdmin()`. Side-effect: `pkg/scoreboards.TopUsers` had no `is_bot` filter — the hardcoded list was hiding that gap. Filter added here. ([#674])
- **Persist `is_bot` in `save()`, add `SetBot` helper.** `save()` now includes `is_bot` in its UPDATE map (previously only `last_seen`, `num_visits`, `miles` — `IsBot` changes never reached the DB). New exported `SetBot(ctx, username, isBot)` flips the flag for any user, logged-in or not; also updates the `LoggedIn` map when the target is online so the change takes effect immediately. Prereq for `!makebot` / `!unbot`. ([#674])

### admin panel

- **Revert the pre-cutover stream-preview staging hack.** Drops the `IsProduction()` if-block in `previewChannel()` that was returning `"adanalife_staging"` on prod-1 so the panel embed showed the live staging stream until prod was actually broadcasting (originally landed in #672, shipped via v2.15.5). After cutover the panel embed shows whatever channel the env is configured for — `adanalife_` on prod, like every other env. Collapses to a one-liner returning `c.Conf.ChannelName`. ([#678])
- **Route dashboard + sibling links over the tailnet.** Switch the dashboards links (traefik, hubble) and the derived OBS sibling link from `*.whereisdana.today` to the Tailscale K8s operator's `*-{prod,stage}.tail020deb.ts.net` names. The whereisdana hosts resolve to LAN IP `192.168.1.200`, which silently dead-ends on client networks sharing the `192.168.1.0/24` range; the operator names are CGNAT (100.x) and don't collide. New `tailnetServiceURL` derives `-prod` / `-stage` from `c.Conf` and returns `""` for dev/local/testing so the link hides on clusters the operator doesn't cover. Auth re-init URLs still use `c.Conf.ExternalURL` (Twitch OAuth `redirect_uri` host — moving them is a bigger change that needs the Twitch app's allowlist updated). ([#684])
- **Reorder panel sections: preview → now playing → controls.** HTML reorder only; no CSS or test changes (`admin_test.go` asserts presence, not order). ([#680])

### obs

- **Shim `xdg-screensaver` to silence per-cycle warnings.** OBS calls `xdg-screensaver suspend` every 30s while streaming to inhibit the desktop screensaver. The freedesktop script can't find a backend in a headless container (`$DISPLAY` unset, no GNOME/KDE session) and exits 2 — OBS then logs `Failed to create xdg-screensaver: 2` at warn level. ~120 wasted fork+exec per pod per hour, all spamming Loki. Drop a no-op shim at `/usr/local/bin/xdg-screensaver` so OBS gets exit 0 and stops complaining. Cleaner than a Loki-side drop filter — the noise never enters the log stream and OBS does no work it doesn't need to. ([#683])

### telemetry

- **Quiet the VLC / onscreens client error cascade.** A single "no route to host" from vlc-server was producing 5 ERROR records (and 5 Sentry exceptions) on one `trace_id`: each HTTP client wrapper logged the operation-specific failure at Error, the transport helper logged it again with the same err, and the downstream `LoadOrCreate` caller logged a third symptom of the same root cause. Demote the transport-helper logs (`vlc-client.get`, `onscreens-client.get`) and the downstream "unable to create Video" to Debug. The wrappers above retain the meaningful "error doing X" Error logs. ([#682])
- **Quiet "OCRing coords skipped!" log to debug.** Fires on every video save without coords (the expected steady state since `ocrCoords` was removed), so logging at Error level spammed Loki and sent a Sentry exception per save. ([#681])

### ci

- **govulncheck workflow.** Scans `go.mod` + the imported call graph against the Go vulnerability database. Tighter signal than CodeQL because it only flags vulns actually reachable from our code paths. Dry-run on develop is clean (0 reachable, 6 latent in `golang.org/x/net` that we don't call). The action bundles its own `actions/checkout` + `setup-go`, so the outer checkout was dropped to avoid a duplicate-Authorization 400. ([#677])

### tests

- **Go fuzz targets for chatbot + helpers parsers.** New fuzz targets for `StripAtSign`, `StateAbbrevToState`, `TitlecaseState`, `FindCommand`, plus per-command fuzzers (`Guess`, `Miles`, `Followage`, `Middle`, `SetBot`) on the existing `newTestApp` scaffolding (with a `recordingIRC` stand-in for `noopIRC`, since `noopIRC.Say` still delegates to the package-level `sayFn` during the App.IRC migration and that path nil-deref'd a Twitch client under fuzz load). Each target runs clean for 10s at ~50k–400k execs/sec. Two bugs surfaced in <1 min of fuzzing and got fixed in the same PR: `StripAtSign("")` panicked on empty input (length check was after the index), and `StateToStateAbbrev("District of Columbia")` returned `""` because `strings.Title` mis-capitalised "of". ([#676])

[#674]: https://github.com/adanalife/tripbot/pull/674
[#675]: https://github.com/adanalife/tripbot/pull/675
[#676]: https://github.com/adanalife/tripbot/pull/676
[#677]: https://github.com/adanalife/tripbot/pull/677
[#678]: https://github.com/adanalife/tripbot/pull/678
[#679]: https://github.com/adanalife/tripbot/pull/679
[#680]: https://github.com/adanalife/tripbot/pull/680
[#681]: https://github.com/adanalife/tripbot/pull/681
[#682]: https://github.com/adanalife/tripbot/pull/682
[#683]: https://github.com/adanalife/tripbot/pull/683
[#684]: https://github.com/adanalife/tripbot/pull/684

## [v2.15.5] — 2026-05-25

Patch release. More admin-panel polish on top of v2.15.4, plus a temporary hack for the prod stream-preview so it shows something live until the broadcasting cutover.

### admin panel

- **Panel layout polish.** Four small changes shipped as one PR: drop the "Groove Salad Classic on SomaFM" link from the audio line (wrapped awkwardly when artist + title was long); wrap "now playing" in a collapsed `<details>` matching the controls / stream-preview shape; move the theme toggle to a new panel footer at the bottom (was crowding the env chip); add an "expand/collapse all" button next to the theme toggle in that footer. ([#671])

### temporary

- **prod-1's stream-preview embeds `adanalife_staging` until the broadcasting cutover.** `previewChannel()` returns `adanalife_staging` when `c.Conf.IsProduction()`, else `ChannelName`. Pure code-side hack — no infra/env-var change. The broadcaster link in the accounts line still uses `Channel` (= `adanalife_`) so click-through goes to the right Twitch page; only the iframe diverges. **Revert at cutover** by deleting the if-block in `pkg/server/admin.go`'s `previewChannel()`. ([#672])

[#671]: https://github.com/adanalife/tripbot/pull/671
[#672]: https://github.com/adanalife/tripbot/pull/672

## [v2.15.4] — 2026-05-25

Patch release. Admin panel polish — the page grows a collapsible stream-preview, picks up system-aware light/dark, learns to refresh itself on iOS Home Screen reopens, and shows what's playing on the SomaFM background-audio source. Also fixes a real bug: obs-server's `POST /admin/shutdown` was returning 500 because Flask's worker-thread request handlers can't call `signal.setitimer()` — the "restart OBS" button now actually works.

### admin panel

- **Collapsible stream-preview with lazy-loaded Twitch embed.** New `<details>` between "now playing" and "dashboards", default closed. On expand, an iframe loads the Twitch player muted; on collapse, the iframe points at `about:blank` so the player stops and bandwidth doesn't keep flowing. The embed's `parent=` parameter is derived from `r.Host` so it matches whichever way the operator reached the panel (public Ingress FQDN or tail*.ts.net via the tailnet). ([#669])
- **Light/dark mode toggle.** CSS custom properties for the foundational palette; `@media (prefers-color-scheme: light)` activates a Tufte-ish off-white palette automatically for OS-light users. A small "◐" text button next to the env chip flips between light/dark and persists the choice in localStorage. Semantic colors (re-auth amber, stream green/red, restart muted, dot up/down) stay hardcoded — they're status signals that should read the same in either theme. ([#667])
- **Collapsible "controls" disclosure (default closed).** The stream toggle moves into a `<details>` element so the page reads calm at-a-glance and the action surfaces hide behind one click. Native browser element, no JS. Restart buttons stay inline in service rows for now; if row clutter grows, they can move into the disclosure later. ([#665])
- **Stop-stream button shrunk + muted.** The old big-red button was sized to dominate when the dangerous action is rarely the right click. Renders muted by default — dark-red background, small border, less padding — and the molly-switch arms it bright red on first click, which is when the visual weight should peak. ([#666])
- **"now playing" audio line.** Below the existing video block, the panel now shows the current track from the SomaFM Groove Salad Classic stream (the OBS background audio source) + a link to the station. Best-effort fetch with a short timeout; quietly omits when SomaFM is slow/down. ([#668])
- **Refresh when iOS Home Screen app returns to focus.** Two-part fix for Dana's stale-on-reopen frustration: `Cache-Control: no-store` headers so Safari doesn't serve a cached page, plus a `visibilitychange` JS listener that reloads when the tab transitions hidden → visible after ≥10s (avoids reload-spam on quick app-switches). ([#664])

### obs

- **obs-server: fix `POST /admin/shutdown` 500.** Confirmed live on prod-1 after v2.15.3 rolled. Flask's dev server runs request handlers in worker threads, and Python's `signal.signal()` / `signal.setitimer()` raise `ValueError` from any thread that isn't the main interpreter thread — so the SIGALRM-based shutdown schedule blew up before sending the kill. Swap to `threading.Timer`: runs in a background thread, fires after the delay, SIGTERMs supervisord. The "restart OBS" button in the admin panel now actually works. ([#663])

[#663]: https://github.com/adanalife/tripbot/pull/663
[#664]: https://github.com/adanalife/tripbot/pull/664
[#665]: https://github.com/adanalife/tripbot/pull/665
[#666]: https://github.com/adanalife/tripbot/pull/666
[#667]: https://github.com/adanalife/tripbot/pull/667
[#668]: https://github.com/adanalife/tripbot/pull/668
[#669]: https://github.com/adanalife/tripbot/pull/669

## [v2.15.3] — 2026-05-24

Patch release. The admin panel grows a restart surface: every backend service exposes `POST /admin/shutdown` that gracefully exits the process so k8s respawns the pod, and the panel itself learns a "restart" button per service with a molly-switch two-click confirm. OBS joins the panel's status table for the first time — a tiny Flask process named `obs-server` (paired with `vlc-server` / `onscreens-server`) runs alongside OBS in the same pod and exposes the same `/health/ready` + `/version` + `/admin/shutdown` shape the Go services use. Small chatbot polish on the side: `!timewarp` gets a 500ms lead-in before the overlay fires so the cover starts opaque on the right frame.

### admin panel

- **`POST /admin/shutdown` across all three Go servers.** Shared `httpmw.ShutdownHandler()` in `pkg/httpmw` alongside the existing `LivenessHandler` / `ReadinessHandler`. Each server registers it under its admin subrouter with one line. Handler responds 202, then asynchronously SIGTERMs the process; the existing graceful-shutdown chain (HTTP drain, telemetry flush, Sentry flush) runs and k8s `restartPolicy: Always` brings the pod back. No kube-API, no ServiceAccount, no client-go — and the same endpoint works whether the admin console is in-tripbot or external. ([#658])
- **OBS in the status table + per-service restart buttons.** Status table now lists all four services (tripbot, vlc-server, onscreens-server, obs). Small "restart" button at the end of every row, molly switch via shared inline JS (renamed `armStream` → `armConfirm` so it reads right for both the stream toggle and restart). `POST /admin/restart/{service}` proxies to that service's `/admin/shutdown`; tripbot is the special case — self-restart triggers an in-process shutdown via `httpmw.ShutdownSignal` directly. New `OBS_SERVER_HOST` env points the panel at obs-server. ([#660])

### obs

- **obs-server: Flask process exposing health + version + shutdown.** OBS itself has no HTTP surface (only obs-websocket on :4455), so this small Flask app runs alongside OBS in the same pod and exposes `/health/ready` + `/version` + `POST /admin/shutdown` on :8080 in the same shape the Go services use. POST /admin/shutdown SIGTERMs supervisord (pid 1) so the container exits and k8s respawns. Flask was picked because the image already had a Python venv (obsws-python + websockify); flask==3.1.0 joins the same venv. ([#659], [#661])
- **Renamed admin-shim → obs-server.** Initial PR landed under "admin-shim" naming; renamed for symmetry with `vlc-server` / `onscreens-server` and to stop overloading the `/admin/*` route prefix that's already used as the per-server operator-actions namespace. File / supervisor unit / port name / env var / config field all swept. ([#661])

### chatbot

- **`!timewarp` lead-in.** The cover overlay now starts 500ms before the playback discontinuity so the opaque frame lands on the right side of the cut, not just usually. ([#657])

[#657]: https://github.com/adanalife/tripbot/pull/657
[#658]: https://github.com/adanalife/tripbot/pull/658
[#659]: https://github.com/adanalife/tripbot/pull/659
[#660]: https://github.com/adanalife/tripbot/pull/660
[#661]: https://github.com/adanalife/tripbot/pull/661

## [v2.15.2] — 2026-05-24

Patch release. The landing page graduates into an admin panel: renamed throughout, with a clickable logo, per-service uptime pills, an env-aware Sentry deep-link, and a stream on/off toggle with a two-click molly switch. Small chatbot + onscreens polish on the side: `!song` no longer attributes SomaFM as the source (Twitch policy hedge), `!somafm` credit command added, and the `!timewarp` cover now reliably spans the inter-clip cut.

### admin panel (formerly "landing page")

- **Rename "landing page" → "admin panel" throughout.** File `pkg/server/landing.go` → `admin.go` (paired test file too), Go symbols (`landingHandler`/`landingTmpl`/`landingData` → `admin*`), test names, and comment references across server / httpmw / instrumentation / cmd / twitch. URL route stays `/`. First sub-step of repurposing the page from a passive status surface into an admin/ops panel — actual admin features land in this release too (see below). ([#651])
- **Logo is a refresh button.** Wraps the A Dana Life mark in `<a href="/">` so clicking it reloads the page. Cheap dynamic-feel UX since the panel refreshes its data per request anyway. ([#650])
- **Per-service uptime pills.** Each row on the status table now carries an "up Xh" pill between name and version. Powered by a new `started_at` (RFC3339) field on each binary's `/version` response — tripbot, vlc-server, and onscreens-server — that callers can derive uptime from locally. ([#654])
- **onscreens-server in the status table.** Third row alongside tripbot and vlc-server, probed via the existing `ONSCREENS_SERVER_HOST` env var (same `/health/ready` + `/version` shape vlc uses). Refactor extracts a `siblingStatus(name, host)` helper so future services drop in as one line. ([#656])
- **Sentry env-link.** Sentry joins the dashboard link strip, pre-filtered to this env's issues via `?environment=<SENTRY_ENVIRONMENT>` (the same tag the sentry-go SDK already stamps on every event). ([#654])
- **OBS stream on/off toggle with molly-switch confirm.** New "stream" section on the panel with a single button reflecting the inverse of OBS's current state: red "stop stream" when active, green "start stream" when idle, "OBS unreachable" when OBS is down. **Molly switch:** first click arms the button (relabel + redden, 5s timer); second click within the window submits. Click-away or timeout disarms. ~20 lines of vanilla JS, no deps. `POST /admin/obs/stream/{start,stop}` calls into the new `pkg/obs` control surface directly — no HTTP hop through vlc-server. Tailnet-only by virtue of the Ingress; no app-layer auth gate. ([#654])

### chatbot

- **`!song` drops the SomaFM/Groove Salad attribution.** Replies with just the track now ("&lt;artist&gt; — &lt;title&gt;") instead of naming the source service — keeps the Twitch music-policy surface smaller while VOD-free playback continues. Adds a separate `!somafm` command that responds with a SomaFM credit + link, so the attribution stays available on demand. ([#648])

### onscreens

- **`!timewarp` cover spans the cut.** Bumped the cover animation another 200ms so it stays opaque through the H.264 discontinuity reliably, not just usually. Server-side hide timer bumped to match. ([#649])

### obs / pkg

- **`pkg/obs` gets a control surface.** New `obs.StartStream(ctx)`, `obs.StopStream(ctx)`, `obs.GetStreamStatus(ctx) (active bool, err error)` package-level functions that each open + close a fresh OBS WebSocket connection. Toggle clicks are rare, so a long-lived shared client isn't worth the coordination cost; `PollStreamingActive`'s existing connection stays as-is for the metrics path. `obs.ErrUnreachable` distinguishes "OBS down" from "OBS replied 'not streaming'", so the admin panel can render a different UX for each. ([#654])
- **`obs.GetStreamStatus` transient errors demoted to `slog.Warn`.** Polling failures that recover within the reconnect window were spamming the error log + Sentry; warn-level keeps them in Loki without the noise. ([#652])

[#648]: https://github.com/adanalife/tripbot/pull/648
[#649]: https://github.com/adanalife/tripbot/pull/649
[#650]: https://github.com/adanalife/tripbot/pull/650
[#651]: https://github.com/adanalife/tripbot/pull/651
[#652]: https://github.com/adanalife/tripbot/pull/652
[#654]: https://github.com/adanalife/tripbot/pull/654
[#656]: https://github.com/adanalife/tripbot/pull/656

## [v2.15.1] — 2026-05-24

Patch release. Closes two operational gaps surfaced in the launch cutover: viewer `!report` chat reports now POST to a Discord webhook (with the existing slog/Sentry path kept as audit trail), and vlc-server gains an in-process RTSP DESCRIBE watchdog that self-heals the silent "OPTIONS 200 / DESCRIBE 500 / OBS sees nothing" failure mode confirmed on both stage-1 and prod-1 over the past few days.

### chatbot

- **`!report` POSTs to a Discord webhook.** New optional `DISCORD_ALERTS_WEBHOOK` env var; when set, `!report` payloads (`**!report** from @user: <message>`) get a goroutined HTTP POST with a 5s timeout so the chat-handler path doesn't block on Discord latency. The existing `slog.ErrorContext` audit line (→ stderr + Sentry via the slog→Sentry handler) stays as the durable trail. Companion to the infra-side webhook plumbing ([adanalife/infra#571](https://github.com/adanalife/infra/pull/571/changes)). ([#645])

### vlc-server

- **RTSP DESCRIBE self-heal watchdog with resume-from-marker.** An in-process watchdog probes `localhost:8554` with an RTSP DESCRIBE every 30s; after 3 consecutive failures (~90s) it persists a resume marker with the currently-playing filename and SIGTERMs the process so supervisord respawns vlc-server. The next process reads the marker and resumes from that file — onscreens-server keeps serving throughout (separate supervisord program, no cycle). Also exposes the same signal as `/health/rtsp` for ad-hoc debugging. Closes a silent failure confirmed on both `stage-1` and `prod-1`: libvlc's RTSP listener answered OPTIONS (200) while DESCRIBE returned 500, `/health/ready` stayed green, but the sout chain was gone and OBS saw nothing for ~2 days. ([#646])

[#645]: https://github.com/adanalife/tripbot/pull/645
[#646]: https://github.com/adanalife/tripbot/pull/646

## [v2.15.0] — 2026-05-23

Minor release. Stream audio comes alive: SomaFM's Groove Salad Classic now feeds the OBS Main scene as background music, and `!song` / `!music` chat commands report the currently-playing track. Three more advertised-but-unwired chat commands (`!twitter`, `!instagram`, `!facebook`, `!youtube` plus short aliases) get registered — `!socialmedia` had been telling viewers about commands that silently did nothing. Smaller polish: the `!timewarp` cover holds long enough to span the inter-clip cut, and the landing page's Hubble link picks the right namespace per environment.

### chatbot

- **`!song` / `!music` commands.** Reports the track currently streaming on the OBS background-audio source. Polls `https://somafm.com/songs/gsclassic.json` with a 30s cache and a stale-fallback on fetch failure; wired through a new `App.NowPlaying` interface so tests don't touch the network. ([#643])
- **`!twitter`, `!instagram`, `!facebook`, `!youtube`.** Inline handlers matching the `!discord` shape; URLs sourced from `website/source/_redirects`. Adds short aliases: `!ig` / `!insta` for Instagram, `!fb` for Facebook, `!yt` for YouTube. Closes a gap surfaced by a pre-launch audit — `!socialmedia` had been advertising these without them being registered. ([#641])

### OBS

- **Groove Salad Classic as the stream's background audio.** New `ffmpeg_source` in the OBS scene template points at SomaFM's mp3-128 Icecast mirror, routed to audio Track 1 (Twitch's track) and referenced by the Main scene so it plays whenever the scene is live. ([#643])
- **`!timewarp` cover spans the inter-clip cut.** Bumped the cover animation 3.4s → 3.8s with a longer opaque hold so it stays opaque through the H.264 discontinuity instead of fading before the next clip's first frames decode. Server-side hide timer raised to 4.4s to match. On-screen text renamed "TIME WARP" → "TIMEWARP" to match the chat command spelling. ([#642])

### tripbot

- **Landing page Hubble link picks the namespace per environment.** Previously hardcoded to `?namespace=prod-1`; now derived from `ENV` (production → prod-1, staging → stage-1) so the stage-1 landing page deep-links into stage-1's flow view. ([#639])

[#639]: https://github.com/adanalife/tripbot/pull/639
[#641]: https://github.com/adanalife/tripbot/pull/641
[#642]: https://github.com/adanalife/tripbot/pull/642
[#643]: https://github.com/adanalife/tripbot/pull/643

## [v2.14.0] — 2026-05-22

Minor release. tripbot's readiness probe is decoupled from the Twitch connection: the pod stays in the Service — and the landing page + OAuth endpoints stay reachable — even when the bot is offline, so the page used to re-auth a disconnected bot is no longer 503'd by the very outage it fixes. The landing page now surfaces re-auth links when a token is missing/expired, and stale tokens self-heal — on an IRC/Helix auth failure the bot re-reads `oauth_tokens` from the DB, so a freshly bootstrapped token is picked up without a restart. Also ships a `!followage` command, an "is this live" alias, and full EventSub event logging.

### tripbot

- **Readiness decoupled from the Twitch connection.** `/health/ready` now returns 200 as soon as the HTTP server is up (via shared `pkg/httpmw` `LivenessHandler` / `ReadinessHandler` / `ReadyCheck` helpers, run with no checks for tripbot). Previously it gated on the Twitch IRC connection, so a disconnected single-replica pod was pulled from the Service and traefik 503'd every route — including the landing page and `/auth/init`. Chat-connection is now a non-gating signal: a `tripbot_twitch_connected` gauge (1/0) plus the landing-page status row ("in chat" / "not in chat"), so "up but not in chat" is surfaced without taking the pod out of rotation. ([#634])
- **OAuth re-auth links on the landing page.** When the bot or broadcaster token is missing or expired, the landing page renders an "action needed: re-authenticate" callout with a "Sign in as `<login>`" button per account, linking to `/auth/init`. `pkg/twitch.AccountsNeedingReauth` reports which accounts need it (broadcaster only when it's a distinct identity), read under the token lock with no DB/network call. Reachable over the internal Ingress (and Tailscale off-LAN); the logged `reauth_url` stays as a backup. ([#635])
- **Hubble dashboard link opens straight to the prod-1 namespace.** Appends `?namespace=prod-1` to the landing page's Hubble link so it lands in the flow view instead of the namespace picker (the Hubble UI is a single prod-zone install shared across envs). ([#630])

### Twitch

- **Self-heal stale tokens by re-reading the DB on auth failure.** Tokens were read once at boot and never re-read, so after a DB restore (stale `oauth_tokens` row) the pod 401'd indefinitely until a manual `rollout restart`. Now `twitch.Reauth(ctx, account)` forces a refresh via the stored `refresh_token` (skipping the 30-minute pre-expiry window) then re-reads the row, treating the DB as source of truth. The IRC connect loop calls it on `ErrLoginAuthenticationFailed`, and `checkHelixResp` calls it on a 401 (with `account=""` opting out the app-token and mid-bootstrap calls; 403 scope-loss is unaffected). A token written by `auth:bootstrap` — or by the landing-page re-auth click — is now picked up within one retry cycle, no restart. ([#636])
- **Log every received EventSub event via `OnRawEvent`.** A catch-all handler in `pkg/eventsub` logs every received notification (`type` + `message_id` + raw payload), whether or not a typed handler is wired for it — an observability-first step toward designing per-event chat shouts from real Loki data. Typed events (follow, subscribe) get both the raw log line and their handler. ([#633])

### chatbot

- **`!followage` command** (alias `!followtime`). Reports how long a viewer has followed the channel; bare `!followage` = the caller, `!followage @user` looks someone else up. New `twitch.FollowedAt` reads `followed_at` from Helix `GetChannelFollows` on the existing broadcaster-token path; duration rendered with `durafmt` (2 units, matching `!uptime`). Not follower-gated; non-followers get a friendly nudge. ([#632])
- **"is this live" aliased to `!date`.** The natural-language question now routes to the command that answers it (multi-word non-`!` aliases were already supported). ([#631])

[#630]: https://github.com/adanalife/tripbot/pull/630
[#631]: https://github.com/adanalife/tripbot/pull/631
[#632]: https://github.com/adanalife/tripbot/pull/632
[#633]: https://github.com/adanalife/tripbot/pull/633
[#634]: https://github.com/adanalife/tripbot/pull/634
[#635]: https://github.com/adanalife/tripbot/pull/635
[#636]: https://github.com/adanalife/tripbot/pull/636

## [v2.13.1] — 2026-05-21

Patch release. tripbot's Ingress root — previously a bare 404 — now serves a lightweight landing-page dashboard (mostly a phone bookmark): a status overview, the currently-playing clip, the broadcaster/bot accounts, and links to the OBS / Grafana / Traefik / Hubble dashboards. Also throttles the token-poll warning that spammed the logs while a pod waits on re-auth.

### tripbot

- **Landing-page dashboard on the Ingress root.** `/` used to 404; it now renders a small dark status page served by `pkg/server` (the old `catchAllHandler` moves to the router's `NotFoundHandler`, so unknown paths still 404). Shows a status overview — tripbot's own readiness (in-memory) plus a live in-cluster ping of vlc-server's `/health/ready` — with each service's build tag in a far-right column linking to that build's `CHANGELOG.md` at its sha (tripbot's own; vlc-server's fetched from its `/version`). When vlc is healthy it also shows the currently-playing clip (file · state · elapsed, read from `pkg/video`'s in-process value — no extra call) and the in-chat count. The header carries the environment; below it the broadcaster + bot Twitch profiles, then a row of dashboard links (OBS noVNC derived from `EXTERNAL_URL`, plus the prod-zone Grafana / Traefik / Hubble UIs). Logo + favicon are referenced from the website's published assets, not copied in. Sibling pings use a 2s-timeout client so a hung vlc-server can't stall the render. ([#628])

### Logging

- **Throttle the "still waiting for Twitch token" poll warning.** `pollForTwitchToken` still checks `LoadFromDB` every 15s (so a freshly-landed token is picked up promptly), but the WARN now logs at most every 15m instead of on every check (~240/hr → ~4/hr while a pod waits on re-auth); the first poll-failure log is suppressed since boot already logged the re-auth link once. Also appends `login_as=<username>` to the logged reauth URL so it names the exact account to sign in as. ([#627])

[#627]: https://github.com/adanalife/tripbot/pull/627
[#628]: https://github.com/adanalife/tripbot/pull/628

## [v2.13.0] — 2026-05-21

Minor release. tripbot now boots degraded instead of crashlooping when Twitch or its OAuth token is unavailable, and surfaces a clickable re-auth link in the logs so recovery is a click rather than a CLI run. On the streaming side, the inter-clip-flash chase concludes: page-cache priming is reverted (didn't move the metric — the gap is libvlc pausing RTP, not file-open latency), and `!timewarp` instead masks the gap with a full-screen warp animation.

### Resilience

- **Start degraded instead of crashlooping when Twitch/token is unavailable.** Two crash paths fixed so the bot comes up with limited functionality and reports *not-ready* rather than dying. (1) When Twitch is unreachable, helix's `RequestAppAccessToken` returns a nil response that `mytwitch.Client()` dereferenced — now guarded, and the tokenless client isn't cached so callers retry once Twitch returns. (2) After a cluster wipe the `oauth_tokens` row is absent; `loadTwitchToken` no longer `os.Exit(1)`s — the bot starts (HTTP, cron, DB-backed features), reports not-ready, and polls `LoadFromDB` in the background until the row lands, then pushes the token into the IRC client. `/health/ready` returns 503 until the IRC connection is established (flipping via the `OnConnect` callback); `/health/live` stays always-200 so the orchestrator doesn't restart the pod while it waits. tripbot is outbound-only, so readiness is a rollout/status signal, not traffic gating. Also adds `task test:macos`, a host-based (mise, no docker) test runner per the `golang-development-with-mise` ADR. ([#624])
- **Surface the OAuth re-auth URL in the logs when the token is missing/expired.** The boot warning, the 15s "still waiting" poll log, the hourly refresh-failure path, and the IRC `ErrLoginAuthenticationFailed` path all now carry a `reauth_url` attribute pointing at `/auth/init`, plus a `login_as` attribute naming the exact account to sign in as (bot vs broadcaster) — so re-auth across three envs and up to six accounts is a click, not a guess. New `mytwitch.AuthInitURL(account)` helper; the auth-bootstrap CLI logs `login_as` up front too. We log the `/auth/init` indirection URL (no `state`, no secret — asserted by test), not the fully-formed Twitch authorize URL whose CSRF `state` would be stale and would land in Loki/Sentry. Clickable from any on-LAN browser via the Ingress `EXTERNAL_URL` set in [adanalife/infra#557]. ([#625])

### Streaming

- **`!timewarp` masks the inter-clip gap with a full-screen warp animation.** The hard cut to a random clip triggers an OBS H.264 discontinuity that briefly clears the dashcam layer — the flash we'd been chasing at the pipeline level. Rather than fix the pipeline, the reworked `!timewarp` overlay (was a small "Timewarp!" text box) slams up a full-screen opaque charcoal warp tunnel with center-burst speed-lines and bold **TIME WARP** text, *physically covering* the gap, then fades onto the new clip — thematically on-point. Timeline ~3.4s: cover slams opaque (~0.4s) → holds through the jump (~2s) → fades to reveal (~0.6s). chatbot waits 800ms after `ShowTimewarp()` before `PlayRandom()` so the cover is up before the cut; the timewarp browser source goes full-canvas at the profile fps. Every other onscreen's render path is byte-for-byte unchanged. ([#623])
- **Revert vlc-server page-cache priming ([#619]).** Priming did not fix the inter-clip flash. Packet analysis showed the gap is libvlc's `PlayAtIndex` pausing RTP emission during the media switch (RTP-over-TCP on 8554 goes idle, no disconnect) — *not* file-open latency, which is what priming addressed; with the file already warm the gap was unchanged. Removed per the "keep only proven changes" principle (primer goroutine, warm-random pool, config knobs all gone). ([#622])

### OBS

- **Default noVNC to local scaling + autoconnect.** The noVNC `index.html` symlink becomes a redirect to `vnc.html?resize=scale&autoconnect=true&reconnect=true`, so opening the OBS VNC URL scales the 1920×1080 canvas to fit the browser window and connects without a manual click. `resize=scale` (not `remote`) keeps the sway output and OBS composition at 1920×1080 — only the client view scales. ([#621])

[#621]: https://github.com/adanalife/tripbot/pull/621
[#622]: https://github.com/adanalife/tripbot/pull/622
[#623]: https://github.com/adanalife/tripbot/pull/623
[#624]: https://github.com/adanalife/tripbot/pull/624
[#625]: https://github.com/adanalife/tripbot/pull/625
[adanalife/infra#557]: https://github.com/adanalife/infra/pull/557

## [v2.12.0] — 2026-05-19

Minor release. OBS VNC moves into the browser via noVNC, replacing the native-client path that never actually worked with macOS Screen Sharing.app. On the streaming side, the inter-clip flash gets its load-bearing fix (page-cache priming) after the VAAPI-transcode approach from v2.11.3 was backed out.

### OBS

- **Browser VNC via noVNC + websockify.** The image now bundles websockify (in the existing obsws-python venv) and the noVNC v1.7.0 static client on `:6080`, supervised as a fifth program that bridges the browser's WebSocket to wayvnc's `:5900`. Reached through a traefik Ingress (companion [adanalife/infra#520]) rather than a native VNC client. This also retires the v2.11.3 wayvnc `enable_auth=true` work: macOS Screen Sharing.app could never connect to neatvnc regardless, because neatvnc 0.7.1 rejects Apple's `RFB 003.889` version handshake *before* any security type is negotiated (confirmed with a debug wayvnc log + raw RFB probe). wayvnc now runs auth-off (RFB "None", bound to the pod's localhost); the per-pod TLS cert + `VNC_USERNAME`/`VNC_PASSWD` machinery and the unused `openssl` package are removed, and access control moves to the Ingress. ([#618])

### Streaming

- **vlc-server page-cache priming to close the inter-clip flash.** A background primer pre-reads upcoming clips into the kernel page cache so libvlc's next-file open hits warm pages instead of a cold NAS round-trip — the latency that left OBS's `ffmpeg_source` briefly disconnected and flashed the broken-video overlay. Two primers off one 5s poll: sequential (warms the next playlist file once the current clip passes `VLC_PRIME_POSITION_THRESHOLD`, default 0.5) and random (keeps one warmed index ready so `!timewarp` lands on a cached clip). Tunable via `VLC_PRIME_ENABLED` / `VLC_PRIME_POSITION_THRESHOLD` / `VLC_PRIME_BYTES`; on by default, cheap on fast local disks. The load-bearing fix of the flash-fix-v2 pass. ([#619])
- **Revert the v2.11.3 VAAPI transcode stage.** VLC's VAAPI module is decode-only — `venc=avcodec{codec=h264_vaapi}` fails to open without a VAAPI device + hw_frames_ctx VLC doesn't wire up, so on prod-1 the sout chain failed to initialize, VLC's RTSP server returned `DESCRIBE 500`, and OBS couldn't pull the dashcam at all. Backs out the inert config + sout-chain code; flash work continues at the OBS-buffering / page-cache layer instead. Companion infra revert [adanalife/infra#519]. ([#617])

[#617]: https://github.com/adanalife/tripbot/pull/617
[#618]: https://github.com/adanalife/tripbot/pull/618
[#619]: https://github.com/adanalife/tripbot/pull/619
[adanalife/infra#519]: https://github.com/adanalife/infra/pull/519
[adanalife/infra#520]: https://github.com/adanalife/infra/pull/520

## [v2.11.3] — 2026-05-19

Patch release. Real-time follow/sub chat shouts return via an EventSub WebSocket listener (no public ingress needed), replacing the v2.9.1-deleted Helix-Webhooks path. vlc-server gains an optional VAAPI transcode stage that keeps the encoder + RTSP listener warm across clip changes, so OBS stops seeing a disconnect/reconnect cycle at clip boundaries. Also ships the two OBS changes that were documented under v2.11.2 but didn't make that build (a stale-branch slip): `BrowserHWAccel=true` and the wayvnc `enable_auth=true` fix.

### Twitch

- **EventSub WebSocket listener for real-time follow + subscribe events.** New `pkg/eventsub` wraps [joeyak/go-twitch-eventsub/v3](https://github.com/joeyak/go-twitch-eventsub) — `Run(ctx, Config, Handlers)` dials the WS, subscribes, and blocks until ctx done (the library handles `session_reconnect` transparently). `channel.follow` v2 and `channel.subscribe` fire chat shouts; the +1-mile-for-everyone bonus on new subs is preserved from the v2.9.1 helpers. Restores `AnnounceNewFollower` / `AnnounceSubscriber` (deleted in #569), adds `mytwitch.BroadcasterUserAccessToken()`, and `cmd/tripbot.startEventSub(ctx)` kicks it off in a goroutine after the Twitch client is up — skips with a warn (not fatal) if the broadcaster row isn't loaded, so the bot still runs without real-time alerts. First of a 5-PR EventSub series. ([#614])

### Streaming

- **Optional VAAPI transcode stage in vlc-server's sout chain.** New `VLC_SOUT_TRANSCODE` (default `false`) wraps the rtp output in a libavcodec `h264_vaapi` transcode stage; combined with the existing `:sout-keep` it keeps the encoder + RTSP listener warm across libvlc input changes, so OBS's `ffmpeg_source` no longer sees a clip-boundary disconnect/reconnect. Off by default so stage-1 / local hostPath envs (no iGPU) keep the passthrough path; prod-1 flips it on via the overlay env. First of a 3-PR flash-fix-v2 pass (output side here; input-side file-open latency follows). Companion [adanalife/infra#518](https://github.com/adanalife/infra/pull/518) sets `VLC_SOUT_TRANSCODE=true` + pins `VLC_AVCODEC_HW=vaapi` on prod-1. ([#615])
- **`BrowserHWAccel=true` in OBS.** Two-character flip in `infra/docker/obs/config/global.ini`. Originally disabled to work around a Mesa-llvmpipe + arm64-CEF interaction where obs-browser advertised the GL shared-texture path to CEF, CEF inspected shared-context availability, saw llvmpipe (no real device), and early-returned under `sharing_available` in `OnPaint` — dropping every frame. With the v2.11.0/v2.11.1 Wayland + iGPU work in place, OBS's GL is on the iris driver and the shared-texture handoff has real GL on both ends (same DRM-PRIME buffer-sharing ring VAAPI's `_tex` encoder uses). Expected wins: lower CPU per browser source (no more software-paint readback), more consistent 2fps for the 11 browser sources, one fewer CPU↔GPU copy per browser-source frame on the composite hot path. ([#611])
- **wayvnc `enable_auth=true` so the TLS cert from #609 actually engages.** Follow-up: #609 set up the cert/key files and listed them in `wayvnc.cfg` but missed `enable_auth=true`, which the wayvnc(5) man page calls out as the gate for `certificate_file` — without it, wayvnc reads the cfg, sees the cert/key paths, and quietly ignores them. Live RFB probe on the v2.11.1 image still showed only security type 1 (None) offered. This PR renames `wayvnc.cfg` → `wayvnc.cfg.tmpl`, envsubsts it into `$XDG_RUNTIME_DIR/wayvnc.cfg` at pod start, sets `enable_auth=true`, and adds `username`/`password` defaults (`adanalife` / `123456`, mirroring the previous x11vnc setup) since `enable_auth` requires them. `relax_encryption=true` is what actually enables Apple-DH (security type 30) — earlier comment framing was wrong; corrected. ([#612])

[#611]: https://github.com/adanalife/tripbot/pull/611
[#612]: https://github.com/adanalife/tripbot/pull/612
[#614]: https://github.com/adanalife/tripbot/pull/614
[#615]: https://github.com/adanalife/tripbot/pull/615

## [v2.11.2] — 2026-05-19

Patch release. Adds a second OAuth flow that consents the broadcaster account, so subscriber/follower polling stops 401'ing on prod-1 — `GetSubscriptions` authorizes against the broadcaster identity, not the bot, so granting `channel:read:subscriptions` to `tripbot4000` was a no-op.

### Twitch

- **Broadcaster-token OAuth flow.** Splits `Scopes` into `BotScopes` + `BroadcasterScopes`, adds a second `*helix.Client` + in-memory token slot for the broadcaster, and routes `GetSubscribers` / `GetFollowerCount` / `UserIsFollower` through it. `cmd/auth-bootstrap` gains an `--account=bot|broadcaster` flag (with `ForceVerify: true` so Twitch re-prompts between runs), and the Taskfile target runs both legs back-to-back. `RefreshUserAccessToken` cron rotates both rows. Surfaced when prod-1's freshly-bootstrapped tripbot logged `helix GetSubscriptions returned 401: Missing scope: channel:read:subscriptions or channel_subscriptions` — the scope was granted on the bot's token, but the API authorizes against the broadcaster identity. ([#604])

[#604]: https://github.com/adanalife/tripbot/pull/604

## [v2.11.1] — 2026-05-19

Patch release. Fixes the two loose ends from v2.11.0's Wayland refactor: OBS's iGPU acceleration didn't actually engage (wlroots rejected `WLR_RENDER_DRM_DEVICE=/dev/dri/card0` as a primary node and silently fell back to pixman + llvmpipe), and Screen Sharing.app on macOS couldn't connect to wayvnc because wayvnc 0.7 doesn't offer legacy VNC Authentication and Screen Sharing.app refuses connections without an encrypted security type. Also drops the transitional `CurrentVideo` closure on the chatbot `App` now that `Video.Current()` covers it, and adds the first test file for `pkg/video` (0% → 36% statement coverage).

### Streaming

- **`WLR_RENDER_DRM_DEVICE` points at the render node, not card0.** `/dev/dri/card0` → `/dev/dri/renderD128` in `start-sway.sh`. wlroots requires a DRM render node; card0 is the primary (KMS scanout) node, so wlroots rejected it and sway fell back to the pixman software renderer, which cascaded into Mesa EGL on the OBS client failing to create a DRI2 screen and OBS loading llvmpipe. After this fix, sway gets a real DRM device handle, the OBS client picks up the `iris` driver, and the zero-copy `_tex` VAAPI path engages. Mac-dev / stage-1 fallback is preserved — if `/dev/dri/renderD128` doesn't exist, sway uses pixman the same as before. ([#608])
- **wayvnc serves a self-signed TLS cert so Screen Sharing.app can connect.** New `infra/docker/obs/config/wayvnc.cfg` configures wayvnc with `private_key_file` + `certificate_file` paths in `$XDG_RUNTIME_DIR` and `relax_encryption=true`; `entrypoint.sh` generates a 10-year self-signed RSA-2048 cert at pod start (regenerated per pod — debug-only VNC, never reaches the internet); `start-wayvnc.sh` switches from positional args to `--config=`. `openssl` added explicitly to both Dockerfiles. With the cert, wayvnc offers VeNCrypt (the encrypted security type Screen Sharing.app prefers); `relax_encryption=true` keeps the "None" path available for simpler VNC clients. ([#609])

### Chatbot

- **Drop the `CurrentVideo func() video.Video` closure on `App`.** The closure was a transitional seam introduced alongside the `Video` interface and slated for removal once `Video` covered the same surface — `Video.Current()` already does exactly what `CurrentVideo()` did. Migrates the eight `a.CurrentVideo()` callsites in `commands.go` to `a.Video.Current()`; `newTestApp(vid)` now wires `&recordingVideo{Vid: vid}` for `Video` instead of also stashing a separate `CurrentVideo` closure. Net: one fewer field on `App`, one fewer thing for test fixtures to wire up, a single interface seam for "the currently-playing video." ([#602])

### Internals

- **First test file for `pkg/video`.** Now that `*Player` is constructable (per v2.11.0 #600), its `GetCurrentlyPlaying` state machine — vid-transition detection, `timeStarted` reset on transition, GPS-image toggle based on the new vid's `Flagged` field — can be exercised against `httptest`-backed `*Client` instances and a sqlmock-backed `gorm.DB`. Coverage on `pkg/video` goes from 0% → 36% of statements. ([#607])

[#602]: https://github.com/adanalife/tripbot/pull/602
[#607]: https://github.com/adanalife/tripbot/pull/607
[#608]: https://github.com/adanalife/tripbot/pull/608
[#609]: https://github.com/adanalife/tripbot/pull/609

## [v2.11.0] — 2026-05-19

Minor release. Finishes the no-globals refactor across the three remaining packages: `pkg/video` lifts into `*Player`, `pkg/onscreens-client` / `pkg/vlc-client` lift into `*Client`, and post-refactor polish lands on `vlc-server` / `onscreens-server` (real `Health()`, graceful shutdown ctx, per-test isolation). OBS's display stack swaps from Xvfb + fluxbox + x11vnc to Sway (headless Wayland) + wayvnc + Qt6 Wayland, supervised by supervisord — gets the 1080p60 composite off Mesa llvmpipe (~14 CPU cores) and onto the iGPU's render engine, and unblocks the `_tex` zero-copy variant of the VAAPI encoder. The `!version` chatbot command stops shelling out and reads `/etc/tripbot/version` directly.

### Streaming

- **OBS display stack on Sway headless + wayvnc.** Out: `xvfb x11vnc fluxbox dbus-x11 x11-utils`. In: `sway wayvnc xwayland qt6-wayland supervisor`. New `sway-headless.conf` (one virtual 1920x1200@60Hz output, fullscreen OBS, no decorations), per-service supervisord declarations (sway → wayvnc → obs → browser-refresh, with Wayland-socket-wait logic for the dependents), `entrypoint.sh` keeps the template-rendering responsibilities but `exec`s supervisord instead of launching Xvfb/fluxbox/x11vnc by hand. `healthcheck.sh` ports from `xprop` / `xdpyinfo` to `swaymsg get_tree`. Background: after v2.10.0's VAAPI work, prod-1 measurement showed OBS CPU only dropped 1579% → 1487%; thread breakdown revealed 19 `llvmpipe-*` threads at 65-82% CPU doing the composite in software because Xvfb has no DRI3 / GLX hardware path. `BrowserHWAccel=false` stays for the initial rollout. ([#597])

### Internals

- **`pkg/video`: `*Player` struct, `NewPlayer(onscreens, vlc)`.** Owns the previously package-scoped state (`curVid`, `preVid`, `timeStarted`, `CurrentlyPlaying`) plus its two HTTP clients. `video.CurrentlyPlaying` (var read) becomes `video.CurrentlyPlaying()` (func call) at 6 callsites. `GetCurrentlyPlaying(ctx)` and `CurrentProgress()` keep their existing shapes as thin shims around `defaultPlayer.X()`. ([#600])
- **`pkg/onscreens-client` and `pkg/vlc-client`: `*Client` structs, `New(host) *Client`.** Globals collapse from 2 per package (URL + `httpClient`) to 1 (`defaultClient`) initially, then to 0 once #600 migrates the last callers. Chatbot's `realOnscreens` / `realVLC` adapters hold a constructed `*Client`, wired in `defaultApp`. ([#596], [#600])
- **`pkg/vlc-server` post-refactor polish.** `New()` either returns a fully-constructed `*Server` or `(nil, err)` — never partial. `(s *Server) Health() error` reflects libvlc player state; `/health/ready` returns 503 with the error message when the player isn't running. Shutdown ctx plumbed into `Shutdown()` and `StartStatsPoller`; `main()`'s graceful-shutdown path drains in-flight HTTP requests with a 5s timeout. ([#594])
- **`pkg/onscreens-server` post-refactor polish.** Tests construct a fresh `*Server` per test via `newTestServer(t)`; the `sync.Once` shared-state helper is gone. Shutdown ctx plumbed through `(s *Server) Start(ctx)` → `Shutdown(ctx)` → `http.Server.Shutdown(ctx)`; `gracefulShutdown` drains in-flight HTTP requests with a 5s timeout. ([#595])

### Chatbot

- **`!version` reads `/etc/tripbot/version` directly, drops the shell-out.** Ports `bin/current-version.sh` to native Go in `pkg/chatbot/versionCmd`. The version file is already baked at build time (per #419), so the per-invocation `git remote update` + `git describe` round-trip was wasted work. Falls back to `"dev"` (matching the ldflag default the `/version` HTTP handler uses) when the file is missing. ([#598])

[#594]: https://github.com/adanalife/tripbot/pull/594
[#595]: https://github.com/adanalife/tripbot/pull/595
[#596]: https://github.com/adanalife/tripbot/pull/596
[#597]: https://github.com/adanalife/tripbot/pull/597
[#598]: https://github.com/adanalife/tripbot/pull/598
[#600]: https://github.com/adanalife/tripbot/pull/600

## [v2.10.0] — 2026-05-19

Minor release. Two `pkg/` binaries (onscreens-server, vlc-server) finish their no-globals refactor — each now constructs via `New(Config)` with explicit dependencies, eliminating package-level state and making both binaries unit-testable. OBS flips to Advanced Output mode for VAAPI encode (v2.9.3's VAAPI work shipped via Simple Output, which has no VAAPI branch in OBS 32's `StreamEncoder` switch — silent fallback to x264 under the hood). Test infrastructure: `.env.testing` was missing `ONSCREENS_SERVER_HOST` since #568, blocking 146 test functions across 7 packages in CI; restoring the var lifts coverage from FAIL (0%) to 59.7% in `pkg/chatbot`, 76.0% in `pkg/oauthtokens`, 41.1% in `pkg/server`, and meaningful coverage in `pkg/users`, `pkg/twitch`, `pkg/video`, `pkg/scoreboards`.

### Streaming

- **OBS Advanced Output mode for VAAPI encode.** Flips `infra/docker/obs/config/basic.ini.tmpl` from Simple to Advanced Output, and adds a per-encoder `streamEncoder.json` rendered by `entrypoint.sh` so VAAPI gets the right keys (`vaapi_device`, integer `profile=100` for H264 High, `rate_control=CBR`); x264 keeps its string-shaped profile. OBS 32's Simple Output mode has no VAAPI branch in its `StreamEncoder` switch (verified against [obs-studio v32.1.2](https://github.com/obsproject/obs-studio/blob/32.1.2/frontend/utility/SimpleOutput.cpp)), so `StreamEncoder=ffmpeg_vaapi_tex` silently fell back to x264 under Simple mode — Advanced mode is the actual switch that makes VAAPI engage. ([#589])

### Internals

- **`pkg/onscreens-server`: `New(Config) *Server`, no package globals.** The 7 onscreen singletons become struct fields; handlers become methods; `cmd/onscreens-server` collapses its `Init*` chain into a single `New()`. ([#591])
- **`pkg/vlc-server`: `New(Config) (*Server, error)`, no package globals.** The libvlc handles (`Player`, `Playlist`, `MediaList`, `VideoFiles`) become struct fields; `PlayRandom` / `Shutdown` / `Start` / `StartStatsPoller` and the HTTP handlers become methods; `cmd/vlc-server` collapses its setup chain into a single `New()` plus method calls. ([#592])

### Testing

- **`.env.testing` was missing `ONSCREENS_SERVER_HOST`, silently blocking 146 tests in CI.** `OnscreensServerHost` was added with `required:"true"` in #568, but `.env.testing` wasn't updated; 7 test packages crashed at config init before running any tests. Restoring the placeholder unlocks the suite — per-package coverage now: `pkg/chatbot` 59.7%, `pkg/oauthtokens` 76.0%, `pkg/server` 41.1%, `pkg/users` 20.6%, `pkg/video` 18.3%, `pkg/twitch` 11.3%, `pkg/scoreboards` 2.7%. Also adds a `skipIfDarwin(t)` guard to 7 playback tests so local `go test` on a Mac no-ops them cleanly (the `*Cmd` handlers under test early-return on Darwin via `helpers.RunningOnDarwin()`). ([#590])

[#589]: https://github.com/adanalife/tripbot/pull/589
[#590]: https://github.com/adanalife/tripbot/pull/590
[#591]: https://github.com/adanalife/tripbot/pull/591
[#592]: https://github.com/adanalife/tripbot/pull/592

## [v2.9.3] — 2026-05-19

Patch release. Wires up a meaningful production-side observability surface: cron jobs now emit run-count / duration / last-run-timestamp metrics and recover panics so a single failing job doesn't kill the scheduler goroutine; HTTP handlers count panics per service so a flapping endpoint becomes alertable instead of just log-shaped; the Helix client surfaces its `Ratelimit-*` response headers as gauges so the per-bearer 800-req/min quota is visible before a 429 fires. Kubelet liveness/readiness probes drop to `slog.LevelDebug` to keep the default Info stream usable. VAAPI hardware acceleration lands on both the OBS encode path and the VLC decode path. Inter-clip transitions in the dashcam stream get smoothed. The `!discord` chatbot command stops handing out invites that expire.

### Observability

- **Cron run counts, duration, last-run timestamps, and panic recovery.** `cmd/tripbot/tripbot.go`'s `tracedJob` wrapper now records `tripbot_cron_runs_total{job}`, `tripbot_cron_duration_seconds{job}` (histogram), and `tripbot_cron_last_run_timestamp_seconds{job}` on every completion — enabling alerts like "no successful run in 3× interval" without changing the (no-return-value) cron callback signature. A `defer recover()` also catches panics, logs the stack via slog, increments `tripbot_cron_panics_total{job}`, and marks the span as Error. Previously a panicking job would crash the scheduler goroutine and silently stop running thereafter. ([#586])
- **`tripbot_http_panics_total{service}` counter via a slog-native recovery middleware.** Replaces `negroni.NewRecovery` with `pkg/httpmw.Recovery` across all three web servers (tripbot, vlc-server, onscreens-server). Recovers panics, logs the stack via slog (so it reaches OTel/Loki and Sentry breadcrumbs/events via the existing handler chain), increments the counter, then writes a 500. `sentrynegroni` continues to capture panics on its inner defer before the outer recovery handles cleanup. ([#586])
- **`twitch_helix_rate_limit_remaining` / `_total` gauges.** Wraps the `otelhttp` transport with a `rateLimitRecorder` that reads `Ratelimit-Remaining` and `Ratelimit-Limit` off every Helix response. The 800-req/min per-bearer quota is shared across all calls from the bot's App Access Token, so a single gauge is the right shape — dashboards / alerts see headroom before 429s land. ([#586])
- **`/health/live` and `/health/ready` log at `slog.LevelDebug`.** Adds `pkg/httpmw.SlogLogger`, an slog-native request logger emitting one structured record per HTTP request (method/path/status/bytes/duration/remote) that replaces `negroni.Logger`'s stdlib log output across all three servers. Health-check probes log at Debug instead of Info so kubelet's frequent liveness/readiness polling stops dominating the default log stream; everything else stays at Info. ([#583])

### Streaming

- **VAAPI hardware acceleration on OBS encode + VLC decode.** OBS now uses `libva` for H.264 encoding (offloading from the CPU); VLC defaults `VLC_AVCODEC_HW` from `vdpau_avcodec` to `any` so libvlc picks VAAPI when the Intel `i915` device is reachable. The matching K8s wiring (Intel device plugin, `gpu.intel.com/i915` resource request on both pods) lives in [adanalife/infra#499](https://github.com/adanalife/infra/pull/499). ([#584])
- **Smooth inter-clip transitions in the dashcam stream.** Reduces the visible cut between dashcam clips so the loop reads as a continuous drive rather than a slideshow of segments. ([#582])

### Chatbot

- **`!discord` points at the non-expiring invite.** Previously the chat command handed out invites with TTL, so the link in the chat scrollback would silently 404 after a while. ([#581])

[#581]: https://github.com/adanalife/tripbot/pull/581
[#582]: https://github.com/adanalife/tripbot/pull/582
[#583]: https://github.com/adanalife/tripbot/pull/583
[#584]: https://github.com/adanalife/tripbot/pull/584
[#586]: https://github.com/adanalife/tripbot/pull/586

## [v2.9.1] — 2026-05-18

Patch release. Unifies the error-capture pipeline on `slog` so direct `slog.Error` calls (telemetry init, OBS adapter, etc.) reach Sentry alongside the existing `terrors.Log` sites, with a per-fingerprint cooldown + hourly cap to stay under the free tier. Threads `ctx` through a few more chatbot/users helpers so the per-command gating decisions and miles-math log lines carry `trace_id` linking back to their `chat.command` span. Retires the dead Twitch Helix Webhooks code path — the upstream API was deprecated by Twitch in 2021 and the receive/subscribe surface has been gated off via `DISABLE_TWITCH_WEBHOOKS=true` since the platform cutover; the only behavior actually lost is the live new-follower / new-sub chat shout, queued for re-introduction via EventSub. Pairs with [adanalife/infra#485](https://github.com/adanalife/infra/pull/485) (drops the `DISABLE_TWITCH_WEBHOOKS` env var from the kustomizations).

### Observability

- **Sentry capture routed through a `slog` handler; `terrors.Log` retired in favor of `slog.ErrorContext`.** Adds `sentryEventHandler` + `breadcrumbHandler` to the multiHandler fan-out in `pkg/telemetry`, so every `slog.Error*` reaches Sentry (and lower-severity records arrive as breadcrumbs, zero quota cost so the next event has context attached). `pkg/errors` gains a `BeforeSend` quota guard: dev/testing drop entirely, prod/staging applies a 15-minute per-fingerprint cooldown plus an absolute hourly cap of 20 — worst-case ~96/day per flapping fingerprint, comfortably under Sentry's 5k/month free cap. `terrors.Log` / `terrors.LogContext` remain as thin slog wrappers during the migration; `scoreboards` + `users` already swept to direct `slog.ErrorContext`. Trace correlation continues to flow via the existing `sentry-go/otel` integration. ([#573])
- **`ctx` threaded through chatUser gating + miles math.** `chatUser.HasCommandAvailable` / `HasGuessCommandAvailable` / `Command.checkAccess` / `(User).CurrentMiles` / `sessionMiles` / `insertIntoLeaderboard` / `UpdateLeaderboard` now accept `context.Context`, so the per-command "letting user run..." and "subscriber will get bonus miles" log lines carry `trace_id` linking back to whichever `chat.command` span or cron tick triggered them. Mechanical follow-up to v2.9.0's broader slog-ctx pass. ([#570])

### Cleanup

- **Retire dead Twitch Helix Webhooks code.** The legacy Helix Webhooks API was deprecated by Twitch in 2021 and the subscription endpoint shut down shortly after; the receive + subscribe code in this repo has been gated off via `DISABLE_TWITCH_WEBHOOKS=true` in the kustomization since the platform cutover, so the path has been unreachable. Deletes `pkg/twitch/webhooks.go`, `pkg/server/twitch.go` (+ its tests), the three webhook HTTP handlers + their routes in `pkg/server/handlers.go` / `pkg/server/server.go` (+ their tests), the now-orphaned `AnnounceNewFollower` / `AnnounceSubscriber` helpers in `pkg/chatbot/chatbot.go`, the startup call + 12h cron + helper in `cmd/tripbot/tripbot.go`, and the `DisableTwitchWebhooks` config field + warn block. Sub-list freshness is unaffected — `mytwitch.GetSubscribers` already runs on a 5-minute cron. The matching kustomization-side cleanup lives in [adanalife/infra#485](https://github.com/adanalife/infra/pull/485). ([#569])

## [v2.9.0] — 2026-05-18

Minor release. `onscreens-server` lifts out of `vlc-server` into its own binary (still co-located in the vlc container via supervisord for now), with a clean `Lookup` / `Snapshot` boundary that makes a future container split a single-PR change. OBS-websocket polling moves from `tripbot` to `vlc-server` where the rest of the OBS data-plane code lives; the `obs_*` gauges' `service.name` flips accordingly. Three conditionally-shown OBS browser sources now shut down CEF when hidden, freeing ~150-250 MB each. `vlc-server` gets graceful HTTP shutdown matching v2.7.1's tripbot-server shape. Optional env vars (`ENV`, `GOOGLE_MAPS_API_KEY`) soft-disable instead of hard-fataling at startup, so local-dev runs work without cluster secrets. Pairs with [adanalife/infra#484](https://github.com/adanalife/infra/pull/484) (k8s Service exposes onscreens-server :8081) and [adanalife/infra#483](https://github.com/adanalife/infra/pull/483) (Grafana stream-health alerts re-labelled for the new `service.name=vlc-server` on OBS metrics).

### Onscreens

- **`onscreens-server` split into its own binary.** Both `vlc-server` and `onscreens-server` run in the same vlc container via supervisord for now; the process split is the boundary that makes a future container split a single-PR change. Onscreens-related HTTP route metrics now carry `service.name=onscreens-server` (same metric names). New `ONSCREENS_SERVER_HOST` env var configures the onscreens-client. ([#568])
- **Clean package boundary via `Lookup` + `Snapshot`.** `pkg/vlc-server` no longer reaches into seven exported singletons inside `pkg/onscreens-server`; the cross-package surface is now `Lookup(slug)`, `Snapshot()`, typed `SlugX` constants, and the existing `Show*` / `Hide*` wrappers. The seven singletons are unexported. ([#567])

### OBS

- **CEF shutdown on conditionally-shown browser sources.** Flag, Middle Text, and Timewarp sources flip `"shutdown": true`, so CEF unloads the child process when they go hidden — frees ~150-250 MB per source. Always-shown sources (leaderboard, both rotators, GPS) keep CEF resident to avoid cold-start delays. Easy-win complement to v2.8.1's FPS cap and hourly refresh. ([#559])
- **OBS-websocket polling moved from tripbot to vlc-server.** `obs_*` gauges keep their names but their `service.name` resource attribute flips from `tripbot` to `vlc-server` — vlc-server already lives on OBS's data plane (RTSP, onscreens HTTP) and is the natural home for OBS-facing integration. Pairs with infra #483 which re-labels the stream-health alerts. ([#564])

### VLC server

- **Graceful HTTP shutdown via signal-derived context.** SIGTERM now waits for in-flight `/onscreens/*`, `/health/*`, `/state.json` requests to finish via `srv.Shutdown(ctx)` with a 15s timeout instead of cutting the connection. Mirrors v2.7.1's tripbot-server shape; resolves the `//TODO: add graceful shutdown` left by [#440]. ([#560])

### DX

- **`vlc-server:{vet,build}:macos` Taskfile targets.** Wraps the macOS libvlc CGO env (`CGO_CFLAGS` / `CGO_LDFLAGS` pointed at `/Applications/VLC.app`, plus the `-Wno-error=incompatible-function-pointer-types` workaround needed by libvlc-go/v3@v3.1.5) so local builds Just Work without per-shell setup. ([#565])

### Config

- **`ENV` defaults to `development`.** Local-dev runs no longer crash with `You must set ENV`. Cluster pods get the value from k8s manifests, so deployed behavior is unchanged. ([#554])
- **`GOOGLE_MAPS_API_KEY` is now optional.** When unset, `helpers.CityFromCoords` / `StateFromCoords` short-circuit with a new `ErrMapsDisabled` sentinel (no failed HTTP, no Sentry noise), the video-import path treats it as steady-state, and `!location` still emits the Google Maps URL (with a blank address). Boot prints a yellow warn alongside the existing webhook-disabled reminder. ([#554])

### Chatbot

- **`!report` ungated.** Flips `RequiresFollow` from true to false on the `!report` Command entry. First-time viewers and lurkers can now use `!report <message>` without first following the channel — report-a-problem shouldn't have a follower gate. ([#561])

### Cleanup

- **Drop stale `infra/k8s/` tree.** Superseded by [adanalife/infra](https://github.com/adanalife/infra). ([#562])
- **Shell-script audit pass.** Documented live callers, removed dead scripts. ([#563])

### Internal

- **More `slog.*Context` migrations.** Twitch helpers (`GetSubscribers`, `GetFollowerCount`, `RefreshUserAccessToken`, subscribe webhook), `users/session.PrintCurrentSession`, `video.GetCurrentlyPlaying`, telemetry init, and cmd-level telemetry shutdown messages all now carry `trace_id` linking Loki records to Tempo spans. Follow-up to v2.8.0's stdlib-log migration ([#552]) and the trace-ctx threading in [#535] / [#547]. ([#566])

## [v2.8.1] — 2026-05-16

Patch release. Stops OBS from OOM-killing itself overnight. The seven onscreen browser sources were rendering at the 60 fps canvas rate even though their content only updates twice a second; CEF leaks a small amount per composited frame, so the per-frame waste compounded to ~10 MB/min of process RSS and tipped the pod over its 3 Gi limit after ~4 h. Caps the render rate at 2 fps and adds an hourly browser-source refresh inside the OBS container entrypoint to drop accumulated CEF state on a fixed cycle, so RSS stays bounded across multi-day uptimes.

### OBS

- **Browser sources pinned to 2 fps.** All seven on-screen browser sources (`GPS`, `Flag`, left/right rotating messages, `leaderboard`, middle text, timewarp) were inheriting the canvas FPS via `fps_custom: false`. The page JS only repolls state every 500 ms, so anything above 2 fps was just rendering identical frames — and CEF leaks per composited frame, so the waste compounded. Observed ~10 MB/min → projected ~0.3 MB/min. ([#555])
- **Hourly browser-source refresh from the entrypoint.** Background `while sleep 3600` loop in `entrypoint.sh` runs `bin/obs-browser-refresh` against the local obs-websocket once an hour, wrapped in `timeout 60` so a wedged call can't stall the cycle. Each refresh reloads the CEF child process per source, dropping accumulated render state and bounding RSS regardless of how long the stream runs. python3 + an `obsws-python` venv added to both `Dockerfile` and `Dockerfile.arm64` (~50 MB image growth). ([#556])

## [v2.8.0] — 2026-05-16

Minor release. Wraps up the chatbot `App` injection pattern (Video, IRC, Sessions now alongside the existing Onscreens / VLC / DB), modernizes the cron scheduler (`robfig/cron` → `gocron/v2`), completes the stdlib `log` → `slog` migration with structured fields, retires the last Stackdriver code path in favor of Loki via OTel, drops Sentry's own tracing (OTel is now the single source of truth — Sentry events link out to Tempo via the SDK's OTel integration), bulk-bumps Go module dependencies, and threads `ctx` through cron-target functions so cron-tick traces nest cleanly.

### Chatbot

- **App injection completed: Video, IRC, Sessions.** `App` now carries injectable interface fields for the remaining external surfaces — `Video` for playback queries, `IRC` for chat output (replacing the `Say()` / `sayFn` indirection), and `Sessions` for user-session bookkeeping. Production wires real implementations via `defaultApp`; tests inject recording/no-op fakes. Unblocks the `jumpCmd` correct-guess test gap that had been deferred from earlier rounds. ([#543], [#544], [#549], [#551])

### Observability

- **`log` → `slog` migration complete.** All ~165 stdlib `log.Println` / `log.Printf` call sites across 44 files migrated to `log/slog` with structured fields (filterable in Grafana Loki), proper levels (warn/error where the message warrants), and `slog.InfoContext` at sites where ctx is already in scope (HTTP handlers, chatbot commands, ctx-aware cron jobs). `aurora` color-wrappers stripped from log calls — ANSI escapes don't belong in structured payloads. `log.Fatal*` kept stdlib (preserves `os.Exit(1)` semantics; still flows through `slogWriter` to OTel). ([#545], [#552])
- **Stackdriver chat-logging retired.** The last GCP Stackdriver code path in `pkg/chatbot/log` now ships chat messages to Loki via OTel logs instead. One fewer GCP dependency; consolidates observability on the OTel stack. ([#540])
- **Sentry tracing dropped; errors link to OTel traces.** Sentry's own tracer is disabled — OTel (otelhttp + otelsql + manual spans → OTLP → Tempo) is the single source of truth for traces. The `sentry-go/otel` integration stamps the active OTel `trace_id` onto captured Sentry events, so error pages link out to their Tempo trace for full request context. ([#550])

### Internal

- **Cron scheduler migrated to `gocron/v2`.** Replaces `robfig/cron@v1.2.0` (untouched since 2021). `gocron/v2`'s `NewTask` accepts `func(context.Context)`, so the scheduler's job ctx is the parent of each tick's span — no more fabricated `context.Background()` in `tracedJob`. Graceful shutdown cancels in-flight job contexts before sentry/telemetry flush. ([#541])
- **Thread `ctx` through cron-target functions.** Follow-up to #541. The eight cron-target functions that took no ctx now accept one; their callers update in step. Cron-tick traces in Tempo now show DB queries (otelsql) and outbound HTTP (otelhttp) nested under the `cron.<name>` span instead of trailing as siblings. ([#547])

### Dependencies

- **Bulk-bump Go modules via `go get -u ./...`.** Refreshes the direct-dep floor without API changes — keeps the upgrade frontier close so the next bump is a smaller round-trip. ([#548])

### DX

- **`task tripbot:auth:bootstrap` rings the console bell before waiting for the Twitch callback.** Audible cue when the flow is ready for the browser sign-in step, so it's harder to miss a paused bootstrap when switching windows. ([#546])

### Cleanup

- **Removed three vestigial files** — `pkg/moments/viewings.go` (never wired up), plus two other unused files. ([#542])

## [v2.7.1] — 2026-05-15

Patch release. End-to-end runtime visibility lands on both ends of the dashcam pipeline (vlc-server + OBS publish OTel gauges to Grafana Cloud), and the chat-command path becomes a single trace tree in Tempo — `chat.command` spans wrap the dispatcher, child SQL queries from GORM and outbound Twitch Helix calls nest underneath. Chatbot picks up an injectable `VLC` dependency, the HTTP server now shuts down gracefully on SIGTERM instead of cutting in-flight requests, and the OBS CI workflow stops building/booting VLC since OBS doesn't actually depend on it for health.

### Observability

- **Pipeline stats: vlc-server and OBS runtime gauges.** vlc-server polls `player.Media().Stats()` every 5s and publishes `vlc_player_input_bitrate` / `demux_bitrate` / `displayed_fps` / `decoded_video_frames` / `displayed_pictures` / `lost_pictures` / `demux_corrupted` / `demux_discontinuity`. OBS's existing WebSocket poll now also calls `General.GetStats` and publishes `obs_active_fps`, `obs_average_frame_render_time_ms`, `obs_cpu_usage_percent`, `obs_memory_usage_mb`, render/output skipped+total frame counters, and stream-side bytes/duration/congestion/reconnecting gauges. Dashboards + alerts in [adanalife/infra#474](https://github.com/adanalife/infra/pull/474). ([#538])

### Tracing

- **`chat.command` span wraps chat-command dispatch; Twitch Helix client gets `otelhttp` transport.** Each IRC message that resolves to a known command now shows up as a span (`command={trigger}` attribute), searchable in Tempo. Helix calls (`GetUsers`, `GetSubscriptions`, `GetChannelFollows`, `GetChannelChatChatters`) now emit outbound HTTP spans, matching the existing `otelhttp` wiring in `pkg/vlc-client` and `pkg/onscreens-client`. ([#533])
- **Thread `context.Context` through `HandlerFunc` and DB helpers.** Builds on #533 so SQL queries (`otelsql`) and outbound HTTP spans nest *under* the `chat.command` span instead of trailing as siblings. `HandlerFunc` grows `ctx`; every `(a *App).xxxCmd` method picks it up; all DB-touching helpers in `pkg/users` / `pkg/scoreboards` / `pkg/events` / `pkg/onscreens-client` grow `ctx` and call `.WithContext(ctx)` on their GORM ops. Server webhook handlers pass `r.Context()`; cron jobs that propagate ctx use a new `tracedJobCtx` wrapper. A single `!miles` is now one tree in Tempo. ([#535])

### Chatbot

- **Inject `VLC` on `App` for testability.** Mirrors the Onscreens injection pattern from #526 — `VLC` interface in `pkg/chatbot/vlc.go` covers the four playback methods chatbot commands use (`PlayRandom`, `PlayFileInPlaylist`, `Skip`, `Back`); `realVLC` delegates to `pkg/vlc-client`. Unlocks the deferred `guessCmd` correct-guess test from #528 — three new tests assert overlay + playback behavior on right-answer / cooldown / new-round transitions via `recordingVLC` + `recordingOnscreens`. ([#536])

### Server

- **Graceful HTTP shutdown via signal-derived context.** `pkg/server/server.go` runs `ListenAndServe` in a goroutine, waits on a context derived from `signal.NotifyContext` on SIGINT/SIGTERM, then calls `srv.Shutdown(ctx)` with a 15s timeout so in-flight requests complete instead of being cut. Resolves the `//TODO: add graceful shutdown` comment that's been there since closed PR #46 (2020). `pkg/vlc-server/server.go` has the same gap and is left as a follow-up. ([#440])

### CI

- **Drop VLC build/start steps from `obs.yml`.** OBS's healthcheck (`pgrep obs`, `xdpyinfo`, safe-mode-modal check) and entrypoint never touch VLC, and RTSP/browser sources retry on missing peers without crashing OBS. Strips the per-arch VLC build, the `up -d vlc`, and the readiness wait; adds `--no-deps` to `up -d obs`. Net `-67/+8` lines, drops ~30s of CI wall time per arch. `docker-compose.yml`'s `depends_on: vlc` is kept so `bin/devenv up obs` still pulls VLC up locally. ([#537])

## [v2.7.0] — 2026-05-15

Minor release. Big one. The database layer migrates from raw `sqlx` to GORM across most packages, Twilio comes out entirely, and three new business metrics land alongside surfaced visibility on non-2xx Twitch Helix responses. Chatbot gains injectable `Onscreens` and `DB` dependencies, unlocking two more rounds of test coverage. Sentry events now carry the build-time version as their `Release` tag, a startup-time `spew.Dump` that was leaking secrets to stdout is gone, and the rotator retires `!survey` while picking up `!discord`.

### Database

- **Migrate to GORM.** Adds `gorm.io/gorm` + `gorm.io/driver/postgres` and `otelgorm` as direct dependencies. `database.GormDB() *gorm.DB` is wired alongside the existing `database.Connection() *sqlx.DB` (sqlx stays for oauthtokens, deferred for a follow-up). `pkg/events`, `pkg/video`, `pkg/users`, and `pkg/scoreboards` move to GORM's fluent API; `sqlx.In()` + `Rebind()` in the leaderboard and scoreboards become native `NOT IN ?` slice expansion. No schema changes — GORM's snake_case convention already matches every existing column name, so old DB dumps import cleanly. ([#499])

### Observability

- **`tripbot_command_duration_seconds` histogram, `tripbot_events_total` counter, `tripbot_scoreboard_writes_total` counter.** Command latency is labelled by `command`; events counter is labelled `login`/`logout`; scoreboard writes labelled by scoreboard name. ([#532])
- **Surface non-2xx Twitch Helix responses with a metric and log.** Closes the silent-failure path that caused the 2026-05-15 incident: `nicklaw5/helix/v2` returns `(resp, nil)` with empty `Data` on 4xx/5xx, so every `pkg/twitch` call site was trusting empty responses and overwriting cached state with zeros. New checks log the offending status + body and increment `tripbot_helix_errors_total{endpoint,status}`. ([#530])
- **Sentry `Release` tag from build-time version.** `terrors.Initialize` now takes a `version string` and sets it as `sentry.ClientOptions.Release`, reusing the `-ldflags "-X main.version=..."`-populated package var that `/version` already exposes (per #419). Sentry events now group by deployed version without any new env-var contract. ([#519])

### Chatbot

- **Inject `Onscreens` and `DB` on `App` for testability.** `Onscreens` interface in `pkg/chatbot/onscreens.go` covers `ShowFlag`, `ShowLeaderboard`, `HideMiddleText`, `ShowMiddleText`, `ShowTimewarp` — `defaultApp` wires `realOnscreens` (delegates to `pkg/onscreens-client`); tests pick from `noopOnscreens` or `recordingOnscreens`. `DB *gorm.DB` lands on `App` for sqlmock-backed command tests. ([#526])
- **DB-backed command tests via sqlmock.** 10 tests covering `lifetimeMilesLeaderboardCmd`, `monthlyMilesLeaderboardCmd`, `topMilesCmd`, `pointsLeaderboardCmd`, `milesCmd`, `guessCmd`, plus their DB-touching helpers. Regex matching on emitted SQL — no postgres service required in CI. ([#528])
- **Overlay-driving paths covered in `flagCmd`, `stateCmd`, `middleCmd`.** Uses the `recordingOnscreens` fake unlocked by #526 to assert each command actually drives the overlay surface it advertises (`ShowFlag` 10s window, `HideMiddleText`, `ShowMiddleText` free-form). ([#531])
- **Retire `!survey`, add `!discord` rotator command.** Both had dangling references in `pkg/onscreens-server/left-rotator.go` and `pkg/config/tripbot/helpers.go` `HelpMessages` with no registry handler. `!survey` is removed entirely (and its three callsite references cleaned up); `!discord` becomes a real inline-registered handler. ([#527])

### VLC

- **`VLC_SERVER_BIND_ADDRESS` replaces the `VLC_SERVER_HOST` requirement.** vlc-server now reads `VLC_SERVER_BIND_ADDRESS` (optional, defaults to `:8080`) for its own bind, so pods boot cleanly without being told their own externally-reachable address. The bot's reading of `VLC_SERVER_HOST` (upstream URL) is unchanged. ([#524])

### Removed

- **Twilio removed entirely.** Supersedes the two stalled lazy-init PRs (#392, #437). Audit found both SMS callsites — `!report` in `pkg/chatbot/commands.go` and the SMS path in `pkg/scoreboards` — had viable Sentry-routed replacements. `pkg/sms` is gone; `TWILIO_*` env vars are dropped from config, deploy manifests, and docs. ([#529])
- **Unconditional `spew.Dump(Conf)` removed from `pkg/config/tripbot` and `pkg/config/vlc-server` `init()`.** The dump leaked `GOOGLE_MAPS_API_KEY`, `TWITCH_CLIENT_SECRET`, `SENTRY_DSN`, Twilio credentials, and `GOOGLE_APPLICATION_CREDENTIALS` to stdout on every process start — ending up in shell scrollback, asciinema recordings, and CI logs. Affects `cmd/tripbot`, `cmd/auth-bootstrap`, `cmd/vlc-server`. ([#523])

### Internal

- **Silence the expected `.env`-missing log in cluster contexts.** `pkg/config/{tripbot,vlc-server}` and `pkg/database` `init()` blocks now only emit the "Error loading .env file / Continuing anyway..." pair when `APP_ENV` is `development` or `testing`. ([#520])
- **Demote `errcheck` reviewdog to `level: warning`.** Stops the noisy red check status caused by libvlc-go's import being unresolvable on the linting runner (no libvlc-dev installed). Matches the existing `revive` job; likely throwaway once super-linter's VALIDATE_GO returns. ([#522])
- **Legacy plain-text `tripbot/todo` repo-root file removed.** Contents long ago routed elsewhere. ([#521])

## [v2.6.4] — 2026-05-15

Patch release. Makes the VLC and OBS containers do less. VLC ditches the local display + X server stack and now streams RTSP only by default; a new `VLC_OUTPUT` env var (`rtsp` | `window` | `both`) keeps the local-window mode available for developers compiling `vlc-server` directly. OBS disables the program preview pane — source rendering happens for the encoder regardless, so the in-app preview was an extra composite onto an Xvfb framebuffer no one watches.

### VLC

- **VLC container runs libvlc headless.** Drops the unused `dst=display` branch from the sout chain (OBS only consumes the RTSP listener) and removes `fluxbox`, `x11vnc`, `xvfb`, `x11-xserver-utils`, `xterm`, and the `vlc` GUI package from the Dockerfile. New `VLC_OUTPUT` env var (`rtsp` | `window` | `both`) lets a developer compile `vlc-server` and run it locally with a preview window. Default `VLC_VOUT` flips from `x11` to `dummy`. ([#516])

### OBS

- **Program preview disabled by default.** Flips `PreviewEnabled` in `user.ini` from `true` to `false`. Source rendering still happens for the stream output regardless; the preview was just an extra composite+blit onto the Xvfb framebuffer. VNC into `:5900` still works for inspecting the OBS UI when debugging. ([#517])

## [v2.6.3] — 2026-05-15

Patch release. Fixes a `\copy` syntax bug in the seed-DB script introduced by #513 in v2.6.2 that caused the seed Job to error before truncating or importing.

### Database

- **Seed-DB `TRUNCATE; \copy` syntax fix.** The combined line shipped in #513 errored with `syntax error at or near "\"` because `\copy` is a psql meta-command and can't share a `-c` string with SQL. Use a heredoc with `--single-transaction` so the two statements run atomically — a failed `\copy` rolls the TRUNCATE back instead of leaving the table empty. Pairs with [adanalife/infra#468](https://github.com/adanalife/infra/pull/468) (wait-for-postgres initContainer in the seed Job manifest). ([#514])

## [v2.6.2] — 2026-05-15

Patch release. Adds Twitch audience gauges (subscribers + followers) and an OBS media-restart helper for reconnecting dropped RTSP sources via WebSocket. Introduces a pre-commit hygiene baseline (ruff + standard fixers). Internal: bumps `go-twitch-irc` to v4, `urfave/negroni` to v3, and `cloud.google.com/go/logging` to v1.18.0.

### Twitch

- **`twitch_subscribers_total` and `twitch_followers_total` OTel gauges.** Emits current channel subscriber and follower counts polled from the Helix API; exposed through the existing OTel meter. ([#497])

### OBS

- **`obs-media-restart` script.** New tool that connects to the OBS WebSocket and reconnects RTSP source inputs — useful when an upstream RTSP stream drops and OBS holds the dead connection. ([#510])

### CI

- **Pre-commit framework + ruff hygiene baseline.** Adds `.pre-commit-config.yaml` covering ruff (Python lint + format), standard pre-commit hooks (trailing whitespace, EOF newline, mixed line endings, AWS-credential / private-key detection), Terraform fmt, and Dockerfile lint. New CI job runs the same set on every PR. ([#511])

### Database

- **Seed-DB skip predicate now ignores tripbot's placeholder rows.** On a fresh cluster, tripbot's `LoadOrCreate` could insert a `flagged=true` placeholder row before the seed Job's init container finished retrying postgres DNS, causing the old `COUNT(*) > 0` skip predicate to false-skip the 4406-row CSV. The script now counts only unflagged rows (`COUNT(*) WHERE NOT flagged`) and `TRUNCATE`s before `\copy` to clear any race-loss placeholders. Pairs with [adanalife/infra#467](https://github.com/adanalife/infra/pull/467). ([#513])

### Internal

- **`go-twitch-irc` v2 → v4.** Major version bump of the Twitch IRC client; import paths updated. ([#485])
- **`urfave/negroni` v1 → v3.** Major version bump of the HTTP middleware library; import paths updated. ([#482])
- **`cloud.google.com/go/logging` v1.4.2 → v1.18.0.** Brings the GCP logging client current; pulls in updated transitive `cloud.google.com/go`, `auth`, `oauth2adapt`, `compute/metadata`, `longrunning`, `s2a-go`, `gax-go/v2`. ([#484])

## [v2.6.1] — 2026-05-15

Patch release. Adds an `obs_streaming_active` OTel gauge tracking live streaming state via WebSocket polling, extends chatbot test coverage to the `App` struct and `middleCmd`, and improves the startup failure message when no Twitch OAuth token is present.

### OBS

- **`obs_streaming_active` OTel gauge.** Polls OBS via WebSocket and emits a gauge reflecting whether the stream is currently live. ([#498])

### Internal

- **Chatbot `App` struct and more command tests.** Test coverage extended to the `App` struct, plus handler tests for `middleCmd`. ([#506], [#507])
- **Clearer log when startup is refused due to missing OAuth token.** The single `log.Fatalf` is split into two lines: one stating the bot is deliberately refusing to start (not crashing), the other giving the exact remediation command. ([#505])

## [v2.6.0] — 2026-05-15

Minor release. Internal improvements only: events table gets an index and a session UUID column (plus a backfill tool for correcting understated historical miles), the chatbot command dispatcher is refactored from a switch statement to a registry map, and the first meaningful test coverage lands for the chatbot package.

### Database

- **`events_username_date` index added.** Covers `(username, date_created)` on the events table; per-user window queries were scanning the full table without it. ([#495])
- **`session_id` UUID column added to `events`.** Login and logout rows for the same session now share a UUID, making session pairs directly queryable rather than inferred by row-number pairing. ([#495])
- **`cmd/backfill-miles` tool added.** Dry-run/apply tool that recomputes historically correct miles from the events ledger and corrects `users.miles` for any user where the stored value is lower than what the event log derives. Found 1,600 understated users in the 2021 prod dump. ([#495])

### Internal

- **Chatbot command dispatch refactored to a registry.** `Command` struct introduced with `Trigger`, `Aliases`, `RequiresFollow`, and `RequiresSubscriber` fields. The `switch` statement in `runCommand` is replaced by two lookup maps (`singleWordLookup`, `multiWordLookup`) built at init time. Routing extracted into `findCommand`; gating logic extracted into `Command.checkAccess` with an injectable `sayFn` for testability. ([#494], [#501])
- **Chatbot package test coverage added.** Registry integrity, `findCommand` routing (single-word, alias, multi-word, inverted-bang, space-separated bang), `checkAccess` gating (follower/subscriber gates with a fake `chatUser`), and command handler tests for `helpCmd`, `uptimeCmd`, `helloCmd`, `kilometresCmd`, and `versionCmd`. ([#500], [#501], [#502], [#503])

## [v2.5.0] — 2026-05-15

Minor release. Enables the OBS WebSocket control plane and adds a `task obs:browser:refresh` command for programmatically refreshing browser sources without VNCing in. Deletes the legacy `/auth/twitch` token-vending HTTP endpoint. Adds `OBS_QUALITY_PRESET` for switching between stream quality profiles, and logs monthly mileage and guess score on user session logout.

### OBS

- **OBS WebSocket server enabled.** The obs-websocket plugin (built into OBS 32) is now seeded at container startup via `plugin_config/obs-websocket/config.json`, with authentication enabled (`OBS_WEBSOCKET_PASSWD`, consistent with `VNC_PASSWD` naming). Port 4455 is exposed in docker-compose and the k8s Service. Unblocks websocket-based healthchecks and streaming-active metrics. ([#491])
- **`task obs:browser:refresh` added.** `bin/obs-browser-refresh` connects to the OBS WebSocket, enumerates all `browser_source` inputs, and calls `PressInputPropertiesButton(refreshnocache)` on each — the programmatic equivalent of right-clicking "Refresh cache of current page" in OBS. Run via `uv run --with obsws-python`; no local install required. ([#491])
- **`OBS_QUALITY_PRESET` env var.** Set to `low` (720p30, 1500 kbps) for dev/staging or `high` (1080p60, 6000 kbps, default) for production. Entrypoint expands the preset into individual encoder params before envsubst. ([#489])

### Users / Sessions

- **Monthly miles and guess score logged on logout.** `users/session` now records each session's `monthly_miles` and `guess_score` to the DB on logout, surfacing per-session contribution data for leaderboards and analytics. ([#443])

### Removed

- **`/auth/twitch` token-vending HTTP endpoint removed.** The endpoint handed out short-lived Twitch tokens over HTTP — replaced by the k8s `auth-bootstrap` Job added in v2.4.3. ([#490])

### Internal

- **`aurora` import migrated from v2 to v3.** ([#486])

## [v2.4.4] — 2026-05-14

Patch release. Centers onscreen rotator text on its grey-box overlay with shrink-to-fit sizing, bakes VLC container configs into the image as discrete files, fixes a noisy `xdg-open` error in headless auth-bootstrap pods, bumps all directly-pinned Go modules to latest compatible versions, and fixes the `obs.yml` CI workflow to build the VLC container from source instead of pulling a stale Docker Hub image.

### Onscreens

- **Rotator text now centers on and fits within its grey-box overlay.** The left and right rotators were centering on the viewport midpoint, causing text to drift over the dashcam footage. `onscreenStyle` gains `AnchorXPx` / `FitWidthPx` / `MinFontSizePx` fields; when `FitWidthPx` is set, text anchors to the grey-box midpoint and shrinks 1px at a time (28→18px floor) until it fits the box width. Left rotator: anchor 282 / fit 564. Right rotator: anchor 456 / fit 369. ([#480])

### VLC Container

- **Static VLC container configs baked into the image.** The four config files (syslog, VNC, VLC supervisord conf, fluxbox startup) are now checked in under `infra/docker/vlc/config/` and `COPY`'d into the image, mirroring the OBS container's pattern. `script/container_startup.sh` slims down to a thin entrypoint. Configs are reviewable as discrete files instead of heredoc strings. ([#442])

### Fixed

- **`auth-bootstrap` no longer errors on `xdg-open` in headless pods.** `OpenInBrowser` is skipped when neither `DISPLAY` nor `WAYLAND_DISPLAY` is set on Linux. The OAuth URL is still printed to stdout so the `task tripbot:auth:bootstrap` port-forward flow works correctly. ([#479])

### CI

- **`obs.yml` now builds the VLC container from source before starting it.** Previously the workflow pulled `adanalife/vlc:latest` from Docker Hub directly. After #442 baked supervisord configs into the image (removing the runtime heredoc writes from `container_startup.sh`), the stale Docker Hub image had no conf.d configs and the thin startup script didn't write them — supervisord launched with no programs, so VLC server never came up. Fix mirrors the OBS build pattern (buildx + GHA cache for amd64, `docker compose build` for arm64). Also adds `infra/docker/vlc/**` and `script/container_startup.sh` to the PR paths trigger so VLC changes fire this workflow on future PRs. ([#487])

### Dependencies

- **Go module bumps (all directly-pinned, API-compatible).** `adrg/libvlc-go/v3` → v3.1.6, `gorilla/mux` → v1.8.1, `jmoiron/sqlx` → v1.4.0, `joho/godotenv` → v1.5.1, `lib/pq` → v1.12.3, `googlemaps/maps` → v1.7.0, `nathan-osman/go-sunrise` → v1.1.0, `sfreiberg/gotwilio` → v1.0.0, `slok/go-http-metrics` → v0.13.0, `unrolled/secure` → v1.17.0. Notable side-effect: deprecated `dgrijalva/jwt-go` replaced by `golang-jwt/jwt` via the gotwilio bump. ([#481])

## [v2.4.3] — 2026-05-13

Patch release. Fixes a long-running IRC disconnect bug where the Twitch connection would fail to re-authenticate after the first token rotation, and adds the `auth-bootstrap` binary to the tripbot image for use by the new k8s bootstrap Job.

### Fixed

- **IRC reconnects no longer fail after the first OAuth token rotation.** `go-twitch-irc` stores the token passed to `NewClient` at construction and replays it on every reconnect. The hourly refresh cron kept tokens fresh in memory and in Postgres, but never updated the IRC client — so any connection drop after the first rotation (~4h post-boot) caused a permanent `login authentication failed` loop. Fix: call `client.SetIRCToken` after each successful refresh (proactive) and on `ErrLoginAuthenticationFailed` in the reconnect loop (recovery).

### Build

- **`auth-bootstrap` binary baked into the tripbot image.** Enables the `task tripbot:auth:bootstrap` k8s Job (infra#450) to run the interactive Twitch OAuth bootstrap in-cluster with direct Postgres access, eliminating the need for a separate DB port-forward.

## [v2.4.2] — 2026-05-12

Patch release. Fixes the k8s seed Job failing with `E: Unable to locate package postgresql-client` by adding `apt-get update` before the install in `seed-db.sh`.

### Build

- **`apt-get update` before `postgresql-client` install in `seed-db.sh`.** The prior release removed the original `apt update` alongside the full `postgresql` package but forgot to keep it for the leaner `postgresql-client` install, causing the seed Job to fail on a stale package index.

## [v2.4.1] — 2026-05-12

Patch release. Prepares the tripbot image for the k8s one-shot DB seed Job: bakes `db/seed/` and `seed-db.sh` into the image, installs `postgresql-client` on-demand at seed time (rather than in the base image), and un-excludes `infra/docker/bin` from `.dockerignore` so the `COPY` in the Dockerfile resolves correctly in CI.

### Build

- **Bake seed data + script into the tripbot image.** `COPY db/seed /seed` and `COPY infra/docker/bin/seed-db.sh /usr/local/bin/seed-db` added to the Dockerfile so the k8s seed Job is self-contained without a volume mount. The companion infra PR wires up the Job and `task tripbot:db:seed`. ([#473])
- **Install `postgresql-client` on-demand in `seed-db.sh`.** Keeps the base image lean — `psql` is only needed for the one-time seed Job, so the script installs `postgresql-client --no-install-recommends` at runtime rather than baking it into every container. ([#473])
- **Un-exclude `infra/docker/bin` from `.dockerignore`.** The ignore file excluded all of `infra/` except `infra/docker/obs`; `infra/docker/bin/seed-db.sh` was invisible to the build context and caused a CI build failure. ([#473])

## [v2.4.0] — 2026-05-12

Minor release. Migrates chatters and follower lookups off deprecated Twitch endpoints onto Helix v2, upgrades the `nicklaw5/helix` dependency from v1 to v2, removes the broken stream-tags shell-script cron (Twitch removed the automated tags API in 2023), pre-bakes Intel VAAPI support into the amd64 OBS image for the incoming mini-PC host, and kills unnecessary LFS fetches in CI that were burning through the 10 GB/mo bandwidth quota.

### Twitch / Authentication

- **Migrate chatters from deprecated TMI endpoint to Helix `GetChannelChatChatters`.** `tmi.twitch.tv/group/user/.../chatters` was the source of the noisiest Sentry errors (2,000+ JSON parse events + 192 timeout events) — the endpoint is defunct and returned garbage or timed out on every call. `pkg/twitch/viewers.go` is now a thin wrapper around `helix.GetChannelChatChatters`; the old raw HTTP client + bespoke JSON struct are gone. `BotID` is lazy-initialized alongside `ChannelID` and used as the `moderator_id`. Two new scopes added to the bot's OAuth token: `moderator:read:chatters` + `moderator:read:followers`. Re-auth bootstrap required on first deploy. ([#471])
- **Migrate follower check from deprecated `GetUsersFollows` to `GetChannelFollows`.** `UserIsFollower` in `twitch.go` called `GetUsersFollows` (removed from the Helix API); replaced with `GetChannelFollows` (`/channels/followers`, scoped by `BroadcasterID` + `UserID`). ([#471])
- **Upgrade `nicklaw5/helix` v1.24.4 → `helix/v2` v2.34.0.** Import path change (`helix` → `helix/v2`) across six files. All existing calls (`GetUsers`, `GetSubscriptions`, `NewClient`, `RequestAppAccessToken`, etc.) have identical signatures in v2. ([#471])
- **Remove stream tags cron + `SetStreamTags` + `bin/set-tags.sh`.** Twitch decommissioned the automated stream tags API on 2023-07-13 in favour of free-form broadcaster-set tags. The 12h cron that shelled out to `set-tags.sh` was causing Sentry errors (`set-tags.sh: no such file or directory`) and doing nothing useful. All three artefacts deleted. ([#471])

### OBS

- **Intel VAAPI driver + `vainfo` added to the amd64 OBS image.** Installs `intel-media-va-driver-non-free` (iHD driver for Gen 11+ iGPUs, Iris Xe / UHD 770) and `vainfo` in preparation for moving OBS to a 12th-gen mini-PC where QuickSync H.264/H.265 encode dramatically reduces CPU vs. software x264. `obs.yml` adds a smoke-test step that runs `vainfo --display drm` in CI to confirm the package set installs cleanly (GHA runners are virtualized and have no real iGPU; `vaInitialize` is expected to fail). arm64 image unchanged. Runtime hookup (device passthrough in the pod spec, encoder flip in OBS scene config) lands in infra when the hardware arrives. ([#469])

### CI

- **Stop pulling Git LFS in CI workflows.** Two workflows were fetching LFS objects unnecessarily: `release.yml` pulled ~432 MB of MP4 per tag on both arches even though `.dockerignore` excludes `assets/video` from the build context; `tripbot.yml` fetched LFS on every PR/push despite the smoke test never touching the dashcam path. Removing both cuts LFS bandwidth usage from ~90% of the 10 GB/mo quota to near zero. Runtime video continues to arrive via the k3d hostpath mount (`infra/k8s/apps/vlc-server/overlays/local/dashcam-hostpath.yaml`). `.gitattributes` `*.MP4` filter-lfs guard preserved for future commits. ([#468])

## [v2.3.2] — 2026-05-11

Patch release. Pre-bakes the OBS arm64 CEF compile into a base image (skipping ~25 min off every OBS PR), fixes four workflow triggers that pointed at a non-existent `main` branch (restoring Coveralls base-build uploads on `develop` and adding `pull_request` scanning to CodeQL), normalizes the OBS scene's seven `browser_source` on-screens to a clean thirds layout (fixes longstanding middle-text clipping), plus a small CI/env hygiene sweep.

### CI

- **OBS arm64 base image (`adanalife/obs-cef-base:arm64-*`).** New `infra/docker/obs/Dockerfile.arm64-base` compiles OBS-from-source against the aarch64 CEF tarball and ships a slim image carrying just `/opt/obs-install/`. New `obs-base.yml` workflow builds and pushes the base on demand. `infra/docker/obs/Dockerfile.arm64` now `FROM`s the base, so the arm64 leg of `obs.yml` drops from ~30 min to ~2 min. Bumping OBS/CEF becomes: edit the base Dockerfile's ARG defaults, push, then bump the `FROM` tag. ([#461])
- **Workflow `push` triggers fixed: `main` → `develop` + `master`.** Testing, super-linter, linting, and CodeQL all listed `main` in their push trigger, but the repo uses `develop` → `master` — so push events on the integration branches never fired these workflows. The visible win: Coveralls now receives a base build on `develop` and PR comments stop reporting "No base build found for commit X on develop." CodeQL additionally gains a `pull_request:` trigger so PRs are scanned (previously only the weekly Thursday cron caught anything). ([#462])
- **`misspell` → `codespell` via super-linter.** Drops the standalone reviewdog `misspell` job from `linting.yml` in favor of super-linter's `SPELL_CODESPELL`; unblocks the v2.3.2 release whose misspell check was failing on pre-existing British "kilometres" usages. New `.codespellrc` ignore list keeps the intentional chat-command typo aliases (`commads`, `quess`, `loacation`, `lcoation`) plus a few project-specific words (`caf`, `nd`, `abitrate`). Bundled with ~20 small typo fixes across 14 files — mostly missing apostrophes in code comments (`can't` / `won't` / `doesn't`), plus a `delimiter` spelling fix in `pkg/helpers/helpers.go` with the in-package caller updated. ([#466])
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

- **Race detector + coveralls.io coverage publishing.** `testing.yml` now runs `go test -v -race -covermode=atomic -coverprofile=coverage.out ./...` and publishes via `jandelgado/gcov2lcov-action` + `coverallsapp/github-action`. Salvaged from closed PR [#126](https://github.com/adanalife/tripbot/pull/126); pairs with the in-progress unit-testing improvements. ([#438])

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

- **`task release:smoke:stage-1`** — combined Taskfile target that applies `k8s/overlays/stage-1`, waits for the four rollouts (tripbot, vlc-server, obs, cloudflared), then hits both local-cluster and `tripbot.whalecore.com` health endpoints. Plus split-out `release:smoke:whalecore` and `release:smoke:local` for re-running just the public or in-cluster checks. Used by the release checklist. ([#402])

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
[#516]: https://github.com/adanalife/tripbot/pull/516
[#517]: https://github.com/adanalife/tripbot/pull/517
[#499]: https://github.com/adanalife/tripbot/pull/499
[#519]: https://github.com/adanalife/tripbot/pull/519
[#520]: https://github.com/adanalife/tripbot/pull/520
[#521]: https://github.com/adanalife/tripbot/pull/521
[#522]: https://github.com/adanalife/tripbot/pull/522
[#523]: https://github.com/adanalife/tripbot/pull/523
[#524]: https://github.com/adanalife/tripbot/pull/524
[#526]: https://github.com/adanalife/tripbot/pull/526
[#527]: https://github.com/adanalife/tripbot/pull/527
[#528]: https://github.com/adanalife/tripbot/pull/528
[#529]: https://github.com/adanalife/tripbot/pull/529
[#530]: https://github.com/adanalife/tripbot/pull/530
[#531]: https://github.com/adanalife/tripbot/pull/531
[#532]: https://github.com/adanalife/tripbot/pull/532
[#440]: https://github.com/adanalife/tripbot/pull/440
[#533]: https://github.com/adanalife/tripbot/pull/533
[#535]: https://github.com/adanalife/tripbot/pull/535
[#536]: https://github.com/adanalife/tripbot/pull/536
[#537]: https://github.com/adanalife/tripbot/pull/537
[#538]: https://github.com/adanalife/tripbot/pull/538
[#540]: https://github.com/adanalife/tripbot/pull/540
[#541]: https://github.com/adanalife/tripbot/pull/541
[#542]: https://github.com/adanalife/tripbot/pull/542
[#543]: https://github.com/adanalife/tripbot/pull/543
[#544]: https://github.com/adanalife/tripbot/pull/544
[#545]: https://github.com/adanalife/tripbot/pull/545
[#546]: https://github.com/adanalife/tripbot/pull/546
[#547]: https://github.com/adanalife/tripbot/pull/547
[#548]: https://github.com/adanalife/tripbot/pull/548
[#549]: https://github.com/adanalife/tripbot/pull/549
[#550]: https://github.com/adanalife/tripbot/pull/550
[#551]: https://github.com/adanalife/tripbot/pull/551
[#552]: https://github.com/adanalife/tripbot/pull/552
[#555]: https://github.com/adanalife/tripbot/pull/555
[#556]: https://github.com/adanalife/tripbot/pull/556
[#559]: https://github.com/adanalife/tripbot/pull/559
[#560]: https://github.com/adanalife/tripbot/pull/560
[#561]: https://github.com/adanalife/tripbot/pull/561
[#562]: https://github.com/adanalife/tripbot/pull/562
[#563]: https://github.com/adanalife/tripbot/pull/563
[#564]: https://github.com/adanalife/tripbot/pull/564
[#565]: https://github.com/adanalife/tripbot/pull/565
[#566]: https://github.com/adanalife/tripbot/pull/566
[#567]: https://github.com/adanalife/tripbot/pull/567
[#568]: https://github.com/adanalife/tripbot/pull/568
[#569]: https://github.com/adanalife/tripbot/pull/569
[#570]: https://github.com/adanalife/tripbot/pull/570
[#573]: https://github.com/adanalife/tripbot/pull/573
[#698]: https://github.com/adanalife/tripbot/pull/698
[#699]: https://github.com/adanalife/tripbot/pull/699
[#700]: https://github.com/adanalife/tripbot/pull/700
[#701]: https://github.com/adanalife/tripbot/pull/701
[#702]: https://github.com/adanalife/tripbot/pull/702
[#707]: https://github.com/adanalife/tripbot/pull/707
[#708]: https://github.com/adanalife/tripbot/pull/708
[#709]: https://github.com/adanalife/tripbot/pull/709
[#710]: https://github.com/adanalife/tripbot/pull/710
[#711]: https://github.com/adanalife/tripbot/pull/711
[#712]: https://github.com/adanalife/tripbot/pull/712
[#713]: https://github.com/adanalife/tripbot/pull/713
[#714]: https://github.com/adanalife/tripbot/pull/714
[#743]: https://github.com/adanalife/tripbot/pull/743
[#735]: https://github.com/adanalife/tripbot/pull/735
[#738]: https://github.com/adanalife/tripbot/pull/738
[#746]: https://github.com/adanalife/tripbot/pull/746
[#747]: https://github.com/adanalife/tripbot/pull/747
[#749]: https://github.com/adanalife/tripbot/pull/749
[#750]: https://github.com/adanalife/tripbot/pull/750
[#751]: https://github.com/adanalife/tripbot/pull/751
[#752]: https://github.com/adanalife/tripbot/pull/752
[#754]: https://github.com/adanalife/tripbot/pull/754
[#716]: https://github.com/adanalife/tripbot/pull/716
[#717]: https://github.com/adanalife/tripbot/pull/717
[#719]: https://github.com/adanalife/tripbot/pull/719
[#720]: https://github.com/adanalife/tripbot/pull/720
[#722]: https://github.com/adanalife/tripbot/pull/722
[#723]: https://github.com/adanalife/tripbot/pull/723
[#736]: https://github.com/adanalife/tripbot/pull/736
[#753]: https://github.com/adanalife/tripbot/pull/753
[#755]: https://github.com/adanalife/tripbot/pull/755
[#757]: https://github.com/adanalife/tripbot/pull/757
[#758]: https://github.com/adanalife/tripbot/pull/758
[#759]: https://github.com/adanalife/tripbot/pull/759
[#764]: https://github.com/adanalife/tripbot/pull/764
[#766]: https://github.com/adanalife/tripbot/pull/766
[#767]: https://github.com/adanalife/tripbot/pull/767
[#768]: https://github.com/adanalife/tripbot/pull/768
[#769]: https://github.com/adanalife/tripbot/pull/769
[#770]: https://github.com/adanalife/tripbot/pull/770
[#777]: https://github.com/adanalife/tripbot/pull/777
[#778]: https://github.com/adanalife/tripbot/pull/778
[#772]: https://github.com/adanalife/tripbot/pull/772
[#773]: https://github.com/adanalife/tripbot/pull/773
[#774]: https://github.com/adanalife/tripbot/pull/774
[#779]: https://github.com/adanalife/tripbot/pull/779
[#780]: https://github.com/adanalife/tripbot/pull/780
[#781]: https://github.com/adanalife/tripbot/pull/781
[#782]: https://github.com/adanalife/tripbot/pull/782
[#744]: https://github.com/adanalife/tripbot/pull/744
[#728]: https://github.com/adanalife/tripbot/pull/728
[#729]: https://github.com/adanalife/tripbot/pull/729
[#787]: https://github.com/adanalife/tripbot/pull/787
[#784]: https://github.com/adanalife/tripbot/pull/784
[#785]: https://github.com/adanalife/tripbot/pull/785
[#788]: https://github.com/adanalife/tripbot/pull/788
[#789]: https://github.com/adanalife/tripbot/pull/789
[#803]: https://github.com/adanalife/tripbot/pull/803
[#804]: https://github.com/adanalife/tripbot/pull/804
[#775]: https://github.com/adanalife/tripbot/pull/775
[#786]: https://github.com/adanalife/tripbot/pull/786
[#792]: https://github.com/adanalife/tripbot/pull/792
[#798]: https://github.com/adanalife/tripbot/pull/798
[#799]: https://github.com/adanalife/tripbot/pull/799
[#796]: https://github.com/adanalife/tripbot/pull/796
[#794]: https://github.com/adanalife/tripbot/pull/794
[#795]: https://github.com/adanalife/tripbot/pull/795
[#793]: https://github.com/adanalife/tripbot/pull/793
[#797]: https://github.com/adanalife/tripbot/pull/797
[#791]: https://github.com/adanalife/tripbot/pull/791
[#802]: https://github.com/adanalife/tripbot/pull/802
[infra #623]: https://github.com/adanalife/infra/pull/623
[infra #654]: https://github.com/adanalife/infra/pull/654
[#790]: https://github.com/adanalife/tripbot/pull/790
[#807]: https://github.com/adanalife/tripbot/pull/807
[#806]: https://github.com/adanalife/tripbot/pull/806
[infra #645]: https://github.com/adanalife/infra/pull/645
[#809]: https://github.com/adanalife/tripbot/pull/809
[#810]: https://github.com/adanalife/tripbot/pull/810
[#811]: https://github.com/adanalife/tripbot/pull/811
[#812]: https://github.com/adanalife/tripbot/pull/812
[#814]: https://github.com/adanalife/tripbot/pull/814
[#815]: https://github.com/adanalife/tripbot/pull/815
[#816]: https://github.com/adanalife/tripbot/pull/816
[#817]: https://github.com/adanalife/tripbot/pull/817
[#818]: https://github.com/adanalife/tripbot/pull/818
[#820]: https://github.com/adanalife/tripbot/pull/820
[#821]: https://github.com/adanalife/tripbot/pull/821
[infra #694]: https://github.com/adanalife/infra/pull/694
[infra #695]: https://github.com/adanalife/infra/pull/695
[#823]: https://github.com/adanalife/tripbot/pull/823
[#824]: https://github.com/adanalife/tripbot/pull/824
[#825]: https://github.com/adanalife/tripbot/pull/825
[#826]: https://github.com/adanalife/tripbot/pull/826
[#827]: https://github.com/adanalife/tripbot/pull/827
[#828]: https://github.com/adanalife/tripbot/pull/828
[#829]: https://github.com/adanalife/tripbot/pull/829
[#830]: https://github.com/adanalife/tripbot/pull/830
[#831]: https://github.com/adanalife/tripbot/pull/831
[#832]: https://github.com/adanalife/tripbot/pull/832
[#834]: https://github.com/adanalife/tripbot/pull/834
[#835]: https://github.com/adanalife/tripbot/pull/835
[#836]: https://github.com/adanalife/tripbot/pull/836
[#837]: https://github.com/adanalife/tripbot/pull/837
[#838]: https://github.com/adanalife/tripbot/pull/838
[#840]: https://github.com/adanalife/tripbot/pull/840
[#841]: https://github.com/adanalife/tripbot/pull/841
[infra #717]: https://github.com/adanalife/infra/pull/717
[#845]: https://github.com/adanalife/tripbot/pull/845
[#846]: https://github.com/adanalife/tripbot/pull/846
[#859]: https://github.com/adanalife/tripbot/pull/859
[#851]: https://github.com/adanalife/tripbot/pull/851
[#854]: https://github.com/adanalife/tripbot/pull/854
[#761]: https://github.com/adanalife/tripbot/pull/761
[#762]: https://github.com/adanalife/tripbot/pull/762
[#763]: https://github.com/adanalife/tripbot/pull/763
[#853]: https://github.com/adanalife/tripbot/pull/853
[#886]: https://github.com/adanalife/tripbot/pull/886
[#861]: https://github.com/adanalife/tripbot/pull/861
[#863]: https://github.com/adanalife/tripbot/pull/863
[#864]: https://github.com/adanalife/tripbot/pull/864
[#865]: https://github.com/adanalife/tripbot/pull/865
[#866]: https://github.com/adanalife/tripbot/pull/866
[#867]: https://github.com/adanalife/tripbot/pull/867
[#868]: https://github.com/adanalife/tripbot/pull/868
[#869]: https://github.com/adanalife/tripbot/pull/869
[#870]: https://github.com/adanalife/tripbot/pull/870
[#871]: https://github.com/adanalife/tripbot/pull/871
[#875]: https://github.com/adanalife/tripbot/pull/875
[#876]: https://github.com/adanalife/tripbot/pull/876
[#877]: https://github.com/adanalife/tripbot/pull/877
[#884]: https://github.com/adanalife/tripbot/pull/884
[#885]: https://github.com/adanalife/tripbot/pull/885
[#887]: https://github.com/adanalife/tripbot/pull/887
[#896]: https://github.com/adanalife/tripbot/pull/896
[#888]: https://github.com/adanalife/tripbot/pull/888
[#892]: https://github.com/adanalife/tripbot/pull/892
[#901]: https://github.com/adanalife/tripbot/pull/901
[#902]: https://github.com/adanalife/tripbot/pull/902
[#903]: https://github.com/adanalife/tripbot/pull/903
[#904]: https://github.com/adanalife/tripbot/pull/904
[#905]: https://github.com/adanalife/tripbot/pull/905
[#906]: https://github.com/adanalife/tripbot/pull/906
[#907]: https://github.com/adanalife/tripbot/pull/907
[#908]: https://github.com/adanalife/tripbot/pull/908
[#909]: https://github.com/adanalife/tripbot/pull/909
[#911]: https://github.com/adanalife/tripbot/pull/911
[#913]: https://github.com/adanalife/tripbot/pull/913
[#919]: https://github.com/adanalife/tripbot/pull/919
[#916]: https://github.com/adanalife/tripbot/pull/916
[#889]: https://github.com/adanalife/tripbot/pull/889
[#920]: https://github.com/adanalife/tripbot/pull/920
[#921]: https://github.com/adanalife/tripbot/pull/921
[#922]: https://github.com/adanalife/tripbot/pull/922
[#923]: https://github.com/adanalife/tripbot/pull/923
[#924]: https://github.com/adanalife/tripbot/pull/924
[#925]: https://github.com/adanalife/tripbot/pull/925
[#926]: https://github.com/adanalife/tripbot/pull/926
