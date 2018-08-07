# docker build -t danalol-stream .
# docker run -v /path/to/videos:/data/inputs -v $(pwd)/outputs:/data/outputs -it danalol-stream:latest

# Pull base image.
FROM ubuntu:16.04

# Set environment variables.
ENV INPUT_DIR /data/inputs
ENV OUTPUT_DIR /data/outputs
ENV HOME /root

RUN mkdir -p $INPUT_DIR $OUTPUT_DIR
VOLUME $OUTPUT_DIR

# Install.
RUN \
  apt-get update && \
  apt-get -y upgrade && \
  apt-get install -y \
    build-essential \
    ffmpeg \
    ruby \
    curl \
    wget \
    git \
    htop \
    man \
    unzip \
    vim && \
  rm -rf /var/lib/apt/lists/*


RUN mkdir /root/bin

# Add files.
ADD smart-shuffle.rb /root
ADD make-vod.sh /root
ADD docker/start.sh /root

# Define working directory.
WORKDIR /root

# Define default command.
ENTRYPOINT ["/root/start.sh"]

