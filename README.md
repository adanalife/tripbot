## Tripbot loves you :robot: :heart:

[![GoDoc](https://godoc.org/github.com/adanalife/tripbot?status.svg)](https://pkg.go.dev/github.com/adanalife/tripbot)
[![Go Report Card](https://goreportcard.com/badge/github.com/adanalife/tripbot)](https://goreportcard.com/report/github.com/adanalife/tripbot)
[![GitHub Super-Linter](https://github.com/adanalife/tripbot/workflows/Super%20Linter/badge.svg)](https://github.com/marketplace/actions/super-linter)
[![Version](https://img.shields.io/github/v/release/adanalife/tripbot?sort=semver&include_prereleases)](https://github.com/adanalife/tripbot/releases)
![Build Status](https://img.shields.io/github/checks-status/adanalife/tripbot/main)
[![License](https://img.shields.io/github/license/adanalife/tripbot)](https://tldrlegal.com/license/mit-license)

This is the source code to [whereisdana.today](http://whereisdana.today), a 24/7 interactive [slow-tv](https://en.wikipedia.org/wiki/Slow_television) art project streaming on Twitch and YouTube.

If you like it, please consider [subscribing](https://dana.lol/prime) to my channel on [Twitch.tv](https://www.twitch.tv/ADanaLife_).
Thanks for watching!

-Dana ([dana.lol](https://dana.lol))


### How it all works

There are two main components built from this repo, each running in its own container: the chatbot itself, which listens for user commands, and an overlay server for on-screen graphics. The dashcam video playback lives in its own repo ([adanalife/playout](https://github.com/adanalife/playout)); the chatbot controls it over NATS and reads the currently-playing clip over HTTP. The scene compositing and streaming to the platforms (Twitch and YouTube) is handled by OBS, which lives in its own repo ([adanalife/obs](https://github.com/adanalife/obs)) and pulls the playback output over RTSP — so the bot and video server can still be split across machines. The chatbot still controls that OBS over its WebSocket (start/stop, health watchdog). The admin UI lives in a separate private repo ([adanalife/tripbot-console](https://github.com/adanalife/tripbot-console)), and platform API calls route through a private [adanalife/platform-gateway](https://github.com/adanalife/platform-gateway) service.

The general flow of information looks like this:

![A diagram showing the different components](assets/infra-diagram.png)

For more detail, check out [Tripbot, the Adventure Robot](https://dana.lol/2020/04/15/tripbot-the-adventure-robot/).


### Developing on the host (quick start)

Day-to-day Go work happens directly on the host. You'll need:

- [mise](https://mise.jdx.dev) — provides the Go toolchain pinned in [`.tool-versions`](.tool-versions)
- [go-task](https://taskfile.dev) — the task runner (`task --list` shows everything)

```bash
# run the unit tests (natively on macOS; plain `task test` runs them in docker)
task test:macos

# or call go directly through mise
mise exec -- go test ./...
mise exec -- go build ./cmd/tripbot
```

### Running the full stack locally

The full application stack (bot + dependencies) runs on the local k3d dev
cluster managed from the [infra repo](https://github.com/adanalife/infra) —
`task k8s:dev:cluster:up` brings it up. See that repo's README for the cluster
lifecycle targets.

For DB-backed tests you only need postgres, which `task test` spins up on demand
via the docker-compose testing stack ([`infra/docker/docker-compose.testing.yml`](infra/docker/docker-compose.testing.yml)) — no manual setup required.

### Changelog

Changelog entries are managed with [towncrier](https://towncrier.readthedocs.io). **Every PR into `main` adds a fragment** describing its user-facing change — a `changelog` CI check enforces this (label a PR `skip-changelog` for dependabot bumps, CI-only tweaks, or pure refactors that warrant no entry).

A fragment is a small markdown file in [`changelog.d/`](changelog.d/) named `<PR-number>.<type>.md`, e.g. `889.fix.md`. Its contents are the entry prose (bold lead-in sentence, then detail — match the existing [`CHANGELOG.md`](CHANGELOG.md) style); the PR link is added automatically.

You won't know the PR number when branching, so run `task changelog:add TYPE=<type>` — it drops a `+`-prefixed placeholder (towncrier's issue-less convention, e.g. `+fix.fix.md`), and the `changelog-number` workflow renames it to `<PR-number>.<type>.md` on first push. No `SKIP_CHANGELOG` dance needed.

```bash
# scaffold one (opens $EDITOR); or just create the file by hand
task changelog:add PR=889 TYPE=fix

# preview the assembled notes
task changelog:preview
```

Types map to the changelog's component sections: `gateway`, `chatbot`, `onscreens`, `playout`, `console`, `fix`, `deploy`, `ci`, `cleanup`, `misc`, plus `summary` (a lead paragraph for the release, named `+summary.summary.md` — no PR number).

### Releases

Releases are trunk-based: [release-please](https://github.com/googleapis/release-please) maintains a standing release PR on `main` with the next version, computed from the conventional commits since the last release. The release PR also carries the collated changelog (built from the `changelog.d/` fragments) and the bumped + re-synthed prod deploy manifests. **Merging the release PR is the release**: it tags `vX.Y.Z`, publishes the GitHub Release, kicks off the multi-arch image builds, and deploys prod (prod-1 autosyncs from `main`).
