# buster is the debian release, c.p. https://askubuntu.com/a/445496
FROM golang:1.14-buster

WORKDIR /go/src/github.com/dmerrick/tripbot

# install dependencies
RUN apt-get update && apt-get install --no-install-recommends -y \
    libtesseract-dev \
    tesseract-ocr \
 && rm -rf /var/lib/apt/lists/*

# create app user
RUN adduser --system --group --disabled-password --no-create-home danalol

# copy over project
#TODO: maybe just copy pkg, cmd, internal, etc?
COPY --chown=danalol:danalol . .

# create symlink to /danalol and give user permissions on it
RUN ln -s /go/src/github.com/dmerrick/tripbot /danalol \
 && chown danalol:danalol /danalol

# set home dir for app user
RUN usermod --home /danalol danalol

# switch to the app user
USER danalol

# build the binary
RUN go build -o bin/tripbot cmd/tripbot/tripbot.go

EXPOSE 8080

ENTRYPOINT ["./bin/tripbot"]
