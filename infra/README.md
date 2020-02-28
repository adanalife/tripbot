## Setting up new machine


#TODO: put this in order, make it pretty

ssh-keygen -t ecdsa -b 521

git clone git@github.com:dmerrick/git-prompt.git
git clone git@github.com:dmerrick/configs.git
#./setup.sh
stow bash tmux vim # ...

# install rvm
# install tmuxinator to global gemset

apt install xsel # pbcopy/pbpaste
# install firefox
# install xfce4-terminal
# install postgresql
# start it, set up db
#install mopidy, mopidy-somafm, mopidy-scrobbler
# configure mopidy
sudo python -m pip install Mopidy-API-Explorer
# install gmpc (music player)

configure pulseaudio
https://docs.mopidy.com/en/latest/running/service/#system-service-and-pulseaudio


# install golang
sudo snap install go --classic
# sudo apt install tesseract-ocr libtesseract-dev

# if you started with ubuntu-server…
sudo apt-get install --no-install-recommends ubuntu-desktop

# set up screen sharing
sudo apt-get install vino
# some magic to be done here, i ended up disabling encryption D:


# install nvidia drivers
sudo add-apt-repository ppa:graphics-drivers/ppa
sudo ubuntu-drivers devices
# find the driver package and install it
# i used non-free cause the version was highest


# install ffmpeg/obs with the script
git clone https://github.com/lutris/ffmpeg-nvenc.git
# you know ffmpeg works if this has results:
ffmpeg -encoders | grep nvenc
# you know obs works if NVENC is in the dropdown
# you might have to use the helper scripts it generates





# sudo apt-get install vlc


# bind ESC to capslock
# https://askubuntu.com/a/365701

