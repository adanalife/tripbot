## Tripbot loves you :robot: :heart:

[![GoDoc](https://godoc.org/github.com/adanalife/tripbot?status.svg)](https://pkg.go.dev/github.com/adanalife/tripbot)
[![Go Report Card](https://goreportcard.com/badge/github.com/adanalife/tripbot)](https://goreportcard.com/report/github.com/adanalife/tripbot)
[![GitHub Super-Linter](https://github.com/adanalife/tripbot/workflows/Super%20Linter/badge.svg)](https://github.com/marketplace/actions/super-linter)
[![Version](https://img.shields.io/github/v/release/adanalife/tripbot?sort=semver&include_prereleases)](https://github.com/adanalife/tripbot/releases)
![Build Status](https://img.shields.io/github/checks-status/adanalife/tripbot/master)
[![License](https://img.shields.io/github/license/adanalife/tripbot)](https://tldrlegal.com/license/mit-license)

This is the source code to [whereisdana.today](http://whereisdana.today), a 24/7 interactive [slow-tv](https://en.wikipedia.org/wiki/Slow_television) art project.

If you like it, please consider [subscribing](https://dana.lol/prime) to my channel on [Twitch.tv](https://www.twitch.tv/ADanaLife_).
Thanks for watching!

-Dana ([dana.lol](https://dana.lol))


### How it all works

There are three main components, each running in its own container: the chatbot itself, which listens for user commands; a VLC-based video server, which manages the currently-playing video; and an OBS container, which composes the scene and streams it to Twitch. OBS pulls the VLC output over RTSP, so the bot and video server can still be split across machines.

The general flow of information looks like this:

![A diagram showing the different components](assets/infra-diagram.png)

For more detail, check out [Tripbot, the Adventure Robot](https://dana.lol/2020/04/15/tripbot-the-adventure-robot/).


### Running tripbot locally

You can use `docker-compose` to run tripbot on your own machine.
It is configured to spin up all of the dependencies for the project.
A helper script ([`bin/devenv`](https://github.com/adanalife/tripbot/blob/main/bin/devenv)) has been created to make the process a little easier.
For example:

```bash
# (optional) create alias for devenv script
alias devenv="$(pwd)/bin/devenv"

# spin up tripbot stack on current machine
devenv up --daemon
# see running containers
devenv ps

# see logs for a specific container
devenv logs tripbot

# shut down everything
devenv down
```


### Finding the deployed build version

Each release stamps its tag and commit SHA into the published images so you can confirm what's running in stage / prod / locally. Three surfaces, in order of preference:

1. **HTTP `/version`** (tripbot bot, vlc-server) — returns JSON `{tag, sha, built_at, dirty}`.
   ```bash
   curl http://<host>:8080/version
   ```
2. **`/etc/tripbot/version` and `/etc/tripbot/sha`** (all three containers) — plain-text files baked in at build time. Useful when HTTP isn't reachable, or for the OBS container which has no HTTP server.
   ```bash
   docker exec <container> cat /etc/tripbot/version
   docker exec <container> cat /etc/tripbot/sha
   ```
3. **Container startup logs** — each container logs its version on the first line of output (`docker logs <container> | head -1`).

The Go binaries also expose `service.version` via OpenTelemetry, so the same value is queryable in Grafana on any traced span / metric / log.

Manual `docker build` invocations default `VERSION=dev` and `SHA=unknown`; only release-tag builds via `.github/workflows/release.yml` populate the real values.

### Other Useful Docs

#### Infra

See [infra/README.md](infra/README.md) for infra setup instructions.

#### Database

See [db/README.md](db/README.md) for database instructions.


