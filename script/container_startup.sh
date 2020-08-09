#!/bin/bash

# this script is the container entrypoint for the OBS container

OUR_IP=$(hostname -i)

# start VNC server (Uses VNC_PASSWD Docker ENV variable)
mkdir -p "$HOME/.vnc" && echo "$VNC_PASSWD" | vncpasswd -f > "$HOME/.vnc/passwd"
vncserver :0 -localhost no -nolisten -rfbauth "$HOME/.vnc/passwd" -xstartup /opt/tripbot/script/x11vnc_entrypoint.sh

echo -e "\n\n------------------ OBS environment started ------------------"
echo -e "\nVNC server started:\n\t=> connect via VNC at vnc://$OUR_IP:5900"
echo -e "\nvlc-server started:\n\t=> connect via http://$VLC_SERVER_HOST/vlc/current\n"
echo -e "\nOBS started:\n\t=> view at https://twitch.tv/$CHANNEL_NAME\n"

if [ ! -z "${STREAM_KEY}" ]; then
  echo -e "\n\nStream is \e[31mLIVE\e[0m!" # red text
fi

if [ -z "$1" ]; then
  # tail all the logs we care about
  # (c.p. https://unix.stackexchange.com/a/195930/202812)
  tail -F /opt/tripbot/log/vlc*.log \
    | awk '/^==> / {a=substr($0, 5, length-8); next}
                 {print a":"$0}'
else
  echo -e "\n\n------------------ EXECUTE COMMAND ------------------"
  echo "Executing command: '$*'"
  exec "$@"
fi
