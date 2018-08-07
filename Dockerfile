# docker run -v /path/to/videos:/data/inputs -v $(pwd)/outputs:/data/outputs -it danalol-stream:latest

# Pull base image.
FROM ubuntu:16.04

RUN mkdir -p /data/inputs /data/outputs
VOLUME /data/outputs

# Install.
RUN \
  apt-get update && \
  apt-get -y upgrade && \
  apt-get install -y \
    build-essential \
    software-properties-common \
    ffmpeg \
    ruby \
    curl git htop man unzip vim wget && \
  rm -rf /var/lib/apt/lists/*


RUN mkdir /root/bin

# Add files.
# ADD root/.bashrc /root/.bashr
# ADD root/.gitconfig /root/.gitconfig
# ADD root/.scripts /root/.scripts
ADD smart-shuffle.rb /root
ADD make-vod.sh /root
ADD docker/start.sh /root

# Set environment variables.
ENV HOME /root

# Define working directory.
WORKDIR /root

# Define default command.
#ENTRYPOINT ["bash"]
ENTRYPOINT ["/root/start.sh"]

