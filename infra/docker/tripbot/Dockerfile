# buster is the debian release, c.p. https://askubuntu.com/a/445496
FROM golang:1.14-buster

WORKDIR /go/src/github.com/dmerrick/danalol-stream

RUN apt-get update && apt-get install --no-install-recommends -y \
    git \
    libtesseract-dev \
    tesseract-ocr \
 && rm -rf /var/lib/apt/lists/*

#TODO: maybe just copy pkg, cmd, internal, etc?
#      because otherwise, stuff like infra and .env.prod could come in
COPY . .

# create symlink to /danalol
RUN ln -s /go/src/github.com/dmerrick/danalol-stream /danalol

# create danalol user
RUN adduser --system --group --disabled-password --no-create-home --home /danalol danalol

#TODO: try switching user first and saving some time on build

# give app user ownership
RUN chown danalol:danalol /danalol
RUN chown -R danalol:danalol /go/src/github.com/dmerrick/danalol-stream

# switch to the app user
USER danalol

# RUN go install github.com/dmerrick/danalol-stream/cmd/tripbot
RUN go build -o bin/tripbot cmd/tripbot/tripbot.go

EXPOSE 8080

CMD ["./bin/tripbot"]
