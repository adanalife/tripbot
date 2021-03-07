# buster is the debian release, c.p. https://askubuntu.com/a/445496
FROM golang:1.16.0-buster

WORKDIR /go/src/github.com/adanalife/tripbot

# create app user
# RUN adduser --system --group --disabled-password --no-create-home danalol

# copy over project
#TODO: maybe just copy pkg, cmd, internal, etc?
COPY . .
# COPY --chown=danalol:danalol . .

# create symlink to /opt/tripbot and give user permissions on it
RUN ln -s /go/src/github.com/adanalife/tripbot /opt/tripbot \
  && mkdir /opt/data #\
  # && chown danalol:danalol /opt/tripbot /opt/data

# set home dir for app user
# RUN usermod --home /opt/tripbot danalol

# switch to the app user
# USER danalol

# build the binary
WORKDIR /opt/tripbot
RUN go build -o bin/tripbot cmd/tripbot/tripbot.go

EXPOSE 8080

CMD ["./bin/tripbot"]
