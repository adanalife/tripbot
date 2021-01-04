## Running kubernetes locally

```bash
brew install k3d
# expose the container's port 80 to localhost:8081
# and mount assets/video to the container's /video
#TODO: pass in rancher/k3s:v1.18.6-k3s1 ??
k3d cluster create adanalife-dev -p 8081:80@loadbalancer --volume $(pwd)/assets/video:/video
# set up kubectl to use this cluster
export KUBECONFIG="$(k3d kubeconfig merge adanalife-dev)"
# create local tripbot deployment
kubectl apply -k infra/k8s/tripbot/stage-1/
curl localhost:8081
```


## Common Tasks

### Tag a release version

All merges to master will bump the semantic version and create a new tag automatically.
By default it will be a patch release, but if you include `#minor` or `#major` in a commit message, it will bump those.

### Backup logs

```bash
mv log/bot.log log/bot.$(date "+%Y%m%d").log
```

### Update a package

```bash
go get -u github.com/nicklaw5/helix
```

### See out-of-date packages

```bash
go get -u github.com/psampaz/go-mod-outdated
go list -u -m -json all | go-mod-outdated
```


### Create SSL certificates using letsencrypt

```bash
EXTERNAL_URL=tripbot.example.com
sudo certbot -d $EXTERNAL_URL --manual --preferred-challenges dns certonly
# use this to verify the DNS change:
dig -t txt _acme-challenge.$EXTERNAL_URL
# copy over the new certs
sudo cp /etc/letsencrypt/live/$EXTERNAL_URL/fullchain.pem infra/certs/$EXTERNAL_URL.fullchain.pem
sudo cp /etc/letsencrypt/live/$EXTERNAL_URL/privkey.pem infra/certs/$EXTERNAL_URL.key
```

To renew certs:

```bash
sudo certbot renew
```

### Create new video manifest

The video manifest is used in the GitHub Actions build process to avoid pulling down big files too often.

```bash
md5sum assets/video/*.MP4 > assets/video/manifest.txt
```


### Create new ffmpeg docker image

We use the [ffpmeg docker file](https://github.com/jrottenberg/ffmpeg/blob/0aed486/docker-images/4.2/nvidia1804/Dockerfile), but with an updated version of freetype.
Specifically, the freetype version matches the one available in `apt`, because the obs-studio build was failing.
It was failing because the version of freetype installed on the system was newer than the one used to build ffmpeg.

```bash
docker build -t danalol/ffmpeg -f infra/docker/obs/Dockerfile.nvidia .
docker push danalol/ffmpeg
```

## Notes on Setting up new machine

These are just notes, this doc needs to be updated with Docker setup instructions.


```bash
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


# OUTDATED
# install ffmpeg/obs with the script
git clone https://github.com/lutris/ffmpeg-nvenc.git
# you know ffmpeg works if this has results:
ffmpeg -encoders | grep nvenc
# you know obs works if NVENC is in the dropdown
# you might have to use the helper scripts it generates





# sudo apt-get install vlc


# bind ESC to capslock
# https://askubuntu.com/a/365701


# get fonts working in obs-studio snap
# https://askubuntu.com/a/943112
```
