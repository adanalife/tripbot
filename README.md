## Tripbot loves you :robot: :heart:

[![GoDoc](https://godoc.org/github.com/adanalife/tripbot?status.svg)](https://pkg.go.dev/github.com/adanalife/tripbot)
[![Go Report Card](https://goreportcard.com/badge/github.com/adanalife/tripbot)](https://goreportcard.com/report/github.com/adanalife/tripbot)
[![GitHub Super-Linter](https://github.com/adanalife/tripbot/workflows/Super%20Linter/badge.svg)](https://github.com/marketplace/actions/super-linter)
[![Version](https://img.shields.io/github/v/release/adanalife/tripbot?sort=semver&include_prereleases)](https://github.com/adanalife/tripbot/releases)
[![Build Status](https://img.shields.io/endpoint.svg?url=https%3A%2F%2Factions-badge.atrox.dev%2Fadanalife%2Ftripbot%2Fbadge&style=flat)](https://actions-badge.atrox.dev/adanalife/tripbot/goto)
[![License: CC BY-NC-SA 4.0](https://img.shields.io/badge/License-CC%20BY--NC--SA%204.0-lightgrey.svg)](https://creativecommons.org/licenses/by-nc-sa/4.0/)

This is the source code to [whereisdana.today](http://whereisdana.today), a 24/7 interactive [slow-tv](https://en.wikipedia.org/wiki/Slow_television) art project.

If you like it, please consider [subscribing](https://dana.lol/prime) to my channel on [Twitch.tv](https://www.twitch.tv/ADanaLife_).
Thanks for watching!

-Dana ([dana.lol](https://dana.lol))


### How it all works

There are two main components: the chatbot itself, which listens for user commands, and a VLC-based video server, which manages the currently-playing video.
They can be run on separate computers.

The general flow of information looks like this:

![A diagram showing the different components](assets/infra-diagram.png)

*Not pictured: a relational database, an MPD-based audio server, and a NAS.*

For more detail, check out [Tripbot, the Adventure Robot](https://dana.lol/2020/04/15/tripbot-the-adventure-robot/).


### Running tripbot locally

You can use `docker-compose` to run tripbot on your own machine.
It is configured to spin up all of the dependencies for the project.
A helper script ([`bin/devenv`](https://github.com/adanalife/tripbot/blob/master/bin/devenv)) has been created to make the process a little easier.
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


### Other Useful Docs

#### Infra

See [infra/README.md](infra/README.md) for infra setup instructions.

#### Database

See [db/README.md](db/README.md) for database instructions.


