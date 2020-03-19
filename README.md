# A Dana Life... Live!

[![GoDoc](https://godoc.org/github.com/dmerrick/danalol-stream?status.svg)](https://godoc.org/github.com/dmerrick/danalol-stream)
[![Go Report Card](https://goreportcard.com/badge/github.com/dmerrick/danalol-stream)](https://goreportcard.com/report/github.com/dmerrick/danalol-stream)
[![Build Status](https://img.shields.io/endpoint.svg?url=https%3A%2F%2Factions-badge.atrox.dev%2Fdmerrick%2Fdanalol-stream%2Fbadge&style=flat)](https://actions-badge.atrox.dev/dmerrick/danalol-stream/goto)
[![License: CC BY-NC-SA 4.0](https://img.shields.io/badge/License-CC%20BY--NC--SA%204.0-lightgrey.svg)](https://creativecommons.org/licenses/by-nc-sa/4.0/)


![](assets/stream-screencap.jpg)

This is the source code to [whereisdana.today](http://whereisdana.today)

If you like it, please follow my channel. Thanks for watching!

-Dana

[dana.lol](https://dana.lol)


## Running tripbot locally

You can use `docker-compose` to run tripbot on your own machine.
A helper script (`[bin/devenv](https://github.com/dmerrick/tripbot/blob/master/bin/devenv)`) has been created to make the process a little easier.
For example:

```bash
# spin up a local tripbot
bin/devenv up
# spin down the tripbot stack
bin/devenv down
# see logs for a specific container
docker logs tripbot_db_1
```


## Other Useful Docs

### Infra

See [infra/README.md](infra/README.md) for infra setup instructions.

### Database

See [db/README.md](#) for database instructions.


