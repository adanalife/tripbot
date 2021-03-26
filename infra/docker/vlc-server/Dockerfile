#TODO: consider using static binaries instead:
# https://johnvansickle.com/ffmpeg/
# this comes from Dockerfile.nvidia
FROM adanalife/obs:v26.1.2

<<<<<<< Updated upstream
=======
# install dependencies
# most of these come from the obs-studio
# install from source instructions
RUN export DEBIAN_FRONTEND=noninteractive \
  && unset LD_LIBRARY_PATH \
  && apt-get update \
  && apt-get install -y --no-install-suggests \
    ack-grep \
    golang-go \
    htop \
    fluxbox \
    net-tools \
    scrot \
    software-properties-common \
    supervisor \
    syslog-ng \
    tigervnc-standalone-server \
    vim \
    vlc \
    vlc-plugin-base \
    wget \
    xterm \
  && apt-get clean -y \
  && rm -rf /var/lib/apt/lists/*

>>>>>>> Stashed changes
# install golang
RUN export DEBIAN_FRONTEND=noninteractive \
  && add-apt-repository ppa:longsleep/golang-backports \
  && apt-get update \
  && apt-get install --no-install-recommends -y \
    golang-go \
  && apt-get clean -y \
  && rm -rf /var/lib/apt/lists/*

# install vdpau driver
#TODO: is this necessary?
RUN apt-get update \
  && apt-get install --no-install-recommends -y \
    vdpau-va-driver \
  && rm -rf /var/lib/apt/lists/*

# for the VNC connection
EXPOSE 5900

# for the HTTP endpoint
EXPOSE 8080

# this allows for custom VNC passwords
ENV VNC_PASSWD=123456

# add menu entries for fluxbox
RUN echo "?package(bash):needs=\"X11\" section=\"ADanaLife\" title=\"OBS Screencast\" command=\"obs\"" >> /usr/share/menu/danalol \
  && echo "?package(bash):needs=\"X11\" section=\"ADanaLife\" title=\"vlc-server\" command=\"cd /opt/tripbot && FONTCONFIG_PATH=/etc/fonts bin/vlc-server\"" >> /usr/share/menu/danalol \
  && echo "?package(bash):needs=\"X11\" section=\"ADanaLife\" title=\"vlc stop\" command=\"supervisorctl stop vlc\"" >> /usr/share/menu/danalol \
  && echo "?package(bash):needs=\"X11\" section=\"ADanaLife\" title=\"vlc start\" command=\"supervisorctl start vlc\"" >> /usr/share/menu/danalol \
  && echo "?package(bash):needs=\"X11\" section=\"ADanaLife\" title=\"obs stop\" command=\"supervisorctl stop obs\"" >> /usr/share/menu/danalol \
  && echo "?package(bash):needs=\"X11\" section=\"ADanaLife\" title=\"obs start\" command=\"supervisorctl start obs\"" >> /usr/share/menu/danalol \
  && echo "?package(bash):needs=\"X11\" section=\"ADanaLife\" title=\"Xterm\" command=\"xterm -ls -bg black -fg white\"" >> /usr/share/menu/danalol \
  && update-menus

# add helpful bash tricks
RUN echo '"\e[A":history-search-backward' >> /root/.inputrc \
  && echo '"\e[B":history-search-forward' >> /root/.inputrc

# copy over project
#TODO: maybe just copy pkg, cmd, internal, etc?
WORKDIR /go/src/github.com/adanalife/tripbot
COPY . .

# create symlink to /opt/tripbot
RUN ln -s /go/src/github.com/adanalife/tripbot /opt/tripbot \
  && mkdir /opt/data

# build the binary
WORKDIR /opt/tripbot
RUN go build -o bin/vlc-server cmd/vlc-server/vlc-server.go

ENTRYPOINT ["/opt/tripbot/script/container_startup.sh"]
