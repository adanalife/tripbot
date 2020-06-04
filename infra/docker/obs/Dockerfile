# this comes from Dockerfile.nvidia
FROM danalol/ffmpeg

# install dependencies
# most of these come from the obs-studio
# install from source instructions
RUN export DEBIAN_FRONTEND=noninteractive \
    && apt-get update \
    && apt-get install -y \
      build-essential \
      checkinstall \
      cmake \
      fluxbox \
      git \
      golang-go \
      libasound2-dev \
      libavcodec-dev \
      libavdevice-dev \
      libavfilter-dev \
      libavformat-dev \
      libavutil-dev \
      libcurl4-openssl-dev \
      libfdk-aac-dev \
      libfontconfig-dev \
      libgl1-mesa-dev \
      libjack-jackd2-dev \
      libjansson-dev \
      libluajit-5.1-dev \
      libmbedtls-dev \
      libnss3-dev \
      libpulse-dev \
      libqt5svg5-dev \
      libqt5svg5-dev\
      libqt5x11extras5-dev \
      libspeexdsp-dev \
      libswresample-dev \
      libswscale-dev \
      libudev-dev \
      libv4l-dev \
      libvlc-dev \
      libvlc-dev \
      libx11-dev \
      libx11-xcb-dev \
      libx264-dev \
      libxcb-randr0-dev \
      libxcb-shm0-dev \
      libxcb-xfixes0-dev \
      libxcb-xinerama0-dev \
      libxcb1-dev \
      libxcomposite-dev \
      libxinerama-dev \
      net-tools \
      pkg-config \
      python3-dev \
      qtbase5-dev \
      scrot \
      software-properties-common \
      swig \
      tigervnc-standalone-server \
      ubuntu-drivers-common \
      vim \
      vlc \
      vlc-plugin-base \
      wget \
      xterm \
      zlib1g-dev \
    && apt-get clean -y \
    && rm -rf /var/lib/apt/lists/*

# install golang v1.14
RUN export DEBIAN_FRONTEND=noninteractive \
  && add-apt-repository ppa:longsleep/golang-backports \
  && apt-get update \
  && apt-get install -y golang-go \
  && apt-get clean -y \
  && rm -rf /var/lib/apt/lists/*

# for the VNC connection
EXPOSE 5900

# for the HTTP endpoint
EXPOSE 8088

# this allows for custom VNC passwords
ENV VNC_PASSWD=123456

# install obs
RUN cd /tmp \
  && git clone https://github.com/obsproject/obs-studio \
  && cd obs-studio \
  && wget https://cdn-fastly.obsproject.com/downloads/cef_binary_3770_linux64.tar.bz2 \
  && tar xjf cef_binary_3770_linux64.tar.bz2 \
  && rm cef_binary_3770_linux64.tar.bz2 \
  && git clone https://github.com/obsproject/obs-browser ./plugins/obs-browser \
  && mkdir -p build \
  && cd build \
  && cmake -DUNIX_STRUCTURE=1 -DBUILD_BROWSER=ON -DCEF_ROOT_DIR="../cef_binary_3770_linux64" .. \
  && make -j2 \
  && make install
  #TODO: possibly add obs-vst?
  # && git clone https://github.com/obsproject/obs-vst ./plugins/obs-vst \

# add menu entries for fluxbox
RUN echo "?package(bash):needs=\"X11\" section=\"ADanaLife\" title=\"OBS Screencast\" command=\"obs\"" >> /usr/share/menu/danalol \
  && echo "?package(bash):needs=\"X11\" section=\"ADanaLife\" title=\"vlc-server\" command=\"cd /opt/tripbot && FONTCONFIG_PATH=/etc/fonts bin/vlc-server\"" >> /usr/share/menu/danalol \
  && echo "?package(bash):needs=\"X11\" section=\"ADanaLife\" title=\"Xterm\" command=\"xterm -ls -bg black -fg white\"" >> /usr/share/menu/danalol \
  && update-menus

# create symlink to /opt/tripbot
RUN ln -s /go/src/github.com/dmerrick/tripbot /opt/tripbot
WORKDIR /opt/tripbot

## create app user
#RUN adduser --system --group --disabled-password --no-create-home danalol
##TODO: merge this with above command
#RUN usermod --home /danalol danalol
#RUN chown -R danalol:danalol /danalol

ENTRYPOINT ["/opt/tripbot/script/container_startup.sh"]
