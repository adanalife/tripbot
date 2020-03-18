# buster is the debian release, c.p. https://askubuntu.com/a/445496
FROM golang:1.14-buster

WORKDIR /go/src/github.com/dmerrick/danalol-stream

RUN apt-get update && apt-get install --no-install-recommends -y \
    libvlc-dev \
 && rm -rf /var/lib/apt/lists/*

#TODO: maybe just copy pkg, cmd, internal, etc?
COPY . .

# create symlink to /danalol
RUN ln -s /go/src/github.com/dmerrick/danalol-stream /danalol

# create danalol user
RUN adduser --system --group --disabled-password --no-create-home --home /danalol danalol

# give app user ownership
RUN chown danalol:danalol /danalol
RUN chown -R danalol:danalol /go/src/github.com/dmerrick/danalol-stream

# switch to the app user
USER danalol

RUN go build -o bin/vlc-server cmd/vlc-server/vlc-server.go

EXPOSE 8080

CMD ["./bin/vlc-server"]
