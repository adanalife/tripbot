# buster is the debian release, c.p. https://askubuntu.com/a/445496
#TODO: we used to have "as go-builder", is that useful?
FROM golang:1.14-buster

WORKDIR /go/src/github.com/dmerrick/danalol-stream

RUN apt-get update && apt-get install -y \
    git \
    libtesseract-dev \
    tesseract-ocr \
 && rm -rf /var/lib/apt/lists/*

#TODO: maybe just copy pkg, cmd, internal, etc?
#      because otherwise, stuff like infra and .env.prod could come in
COPY . .

RUN go install github.com/dmerrick/danalol-stream/cmd/tripbot
# RUN go build -o tripbot .

#TODO: run as a special user
# RUN adduser -S -D -H -h /app appuser
# USER appuser

EXPOSE 8080
