#TODO: consider using static binaries instead:
# https://johnvansickle.com/ffmpeg/
# this comes from Dockerfile.nvidia
FROM adanalife/ffmpeg:v4.2.3

# install dependencies
# most of these come from the obs-studio
# install from source instructions
RUN export DEBIAN_FRONTEND=noninteractive \
  && unset LD_LIBRARY_PATH \
  && apt-get update \
  && apt-get install -y --no-install-suggests \
    ack-grep \
    build-essential \
    checkinstall \
    cmake \
    fluxbox \
    git \
    golang-go \
    grc \
    htop \
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
    supervisor \
    swig \
    syslog-ng \
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

WORKDIR /tmp

ENV OBS_VERSION=26.1.2

# install obs and the obs-browser plugin
#TODO: re-enable browser plugin
RUN git clone --depth 1 --branch $OBS_VERSION https://github.com/obsproject/obs-studio \
  && cd obs-studio \
  && wget https://cdn-fastly.obsproject.com/downloads/cef_binary_3770_linux64.tar.bz2 \
  && tar xjf cef_binary_3770_linux64.tar.bz2 \
  && rm cef_binary_3770_linux64.tar.bz2 \
  && git clone --depth 1 https://github.com/obsproject/obs-browser ./plugins/obs-browser \
  && mkdir -p build \
  && cd build \
  && cmake -DUNIX_STRUCTURE=1 -DBUILD_BROWSER=OFF -DCEF_ROOT_DIR="../cef_binary_3770_linux64" .. \
  && make -j2 \
  && make install
  #TODO: possibly add obs-vst?
  # && git clone https://github.com/obsproject/obs-vst ./plugins/obs-vst \
