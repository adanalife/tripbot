#!/usr/bin/env bash

# enable SSH on the account
sudo systemsetup -setremotelogin on

# generate SSH keypair
mkdir -p ~/.ssh
chmod 700 ~/.ssh
ssh-keygen -t ecdsa -b 521 -C "Created by Dana" -f ~/.ssh/dana
eval `ssh-agent`
ssh-add ~/.ssh/dana

# install homebrew
/usr/bin/ruby -e "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/master/install)"

# install and launch Docker
brew cask install docker

#TODO: this didn't work for some reason
open -a Docker

# install mosh and tmux
brew install mosh tmux

# print the SSH key so we can add it to Github
echo
cat ~/.ssh/dana.pub
sleep 20
echo

mkdir ~/danastuff
cd ~/danastuff

# download the stream project
git clone git clone git@github.com:dmerrick/danalol-stream.git
cd danalol-stream

# build the docker image
docker build -t danalol-stream .

# run the docker container
docker run \
  -v /Volumes/blackbox/Danas_Photos/Dashcam:/data/inputs \
  -v /Volumes/blackbox/outputs:/data/outputs \
  -v /Volumes/blackbox/playlists:/data/playlists \
  -it \
  danalol-stream:latest

# cleanup
# rm ~/.ssh/dana*
# rm -rf ~/danastuff
# uninstall docker
# uninstall homebrew
# disable SSH?

