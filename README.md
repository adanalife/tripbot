# A Dana Life... Live!

[![GoDoc](https://godoc.org/github.com/dmerrick/danalol-stream?status.svg)](https://godoc.org/github.com/dmerrick/danalol-stream)
[![Go Report Card](https://goreportcard.com/badge/github.com/dmerrick/danalol-stream)](https://goreportcard.com/report/github.com/dmerrick/danalol-stream)
[![Build Status](https://img.shields.io/endpoint.svg?url=https%3A%2F%2Factions-badge.atrox.dev%2Fdmerrick%2Fdanalol-stream%2Fbadge&style=flat)](https://actions-badge.atrox.dev/dmerrick/danalol-stream/goto)

![](assets/stream-screencap.jpg)

This is the source code to [whereisdana.today](http://whereisdana.today)

If you like it, please follow my channel. Thanks for watching!

-Dana

[dana.lol](https://dana.lol)


## Install Dependencies

Go should auto-magically pull down all of the required packages when you use `go run` to run something.
You will also need to run:

```
# install tesseract
sudo apt install tesseract-ocr libtesseract-dev
```

To get Streamlabs chat to work on Linux, I ended up using the [obs-linuxbrowser](https://github.com/bazukas/obs-linuxbrowser) plugin for OBS.


### Infra

For more detailed install instructions, see [infra/README.md](infra/README.md)

### Database

See [db/README.md](#) for database instructions.


## Common Tasks

### Backup logs

```
mv log/bot.log log/bot.$(date "+%Y%m%d").log
```

### Start the bot

```
go run cmd/tripbot/tripbot.go
```


### Update a package

```
go get -u github.com/nicklaw5/helix
```

### See out-of-date packages
```
go get -u github.com/psampaz/go-mod-outdated
go list -u -m -json all | go-mod-outdated
```


### Create SSL certificates using letsencrypt
```
sudo certbot -d tripbot.dana.lol --manual --preferred-challenges dns certonly
# use this to verify the DNS change:
dig -t txt _acme-challenge.tripbot.dana.lol
# copy over the new certs
sudo cp /etc/letsencrypt/live/tripbot.dana.lol/fullchain.pem infra/certs/tripbot.dana.lol.fullchain.pem
sudo cp /etc/letsencrypt/live/tripbot.dana.lol/privkey.pem infra/certs/tripbot.dana.lol.key
```

To renew certs:
```
sudo certbot renew
```

### Tag a release version

All merges to master will bump the semantic version and create a new tag automatically.
By default it will be a patch release, but if you include #minor or #major in a commit message, it will bump those.
