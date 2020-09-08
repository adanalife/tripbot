#!/bin/bash

# this script is the container entrypoint for the OBS container

#TODO: set background in /etc/X11/fluxbox/overlay

# don't ipen vncconfig on startup
sed -i '/vncconfig/s/^/#/' /etc/X11/Xvnc-session

mkdir -p $XDG_RUNTIME_DIR
chmod 0700 $XDG_RUNTIME_DIR

mkdir -p /opt/data/run
touch /opt/data/run/{left,right}-message.txt

cat << EOF > /etc/syslog-ng/conf.d/obs-info.conf
@define allow-config-dups 1
filter f_syslog3 { not facility(auth, authpriv, mail) and not filter(f_debug)
  or (message("obs") and message("VLC") and message("update settings"))
  or (message("obs") and message("title: VLC media player"))
  or (message("obs") and message("class: vlc"))
  or (message("obs") and message("Bit depth: 24"))
  or (message("obs") and message("Found proper GLXFBConfig (in 100): yes"));
};
EOF

#mkdir -p /root/.fluxbox
#cat << EOF > /root/.fluxbox/startup
##!/bin/sh

##TODO: not 100% sure what this does
## Change your keymap:
#xmodmap "/root/.Xmodmap"

## set the resolution
#xrandr -s 1920x1200 -r 60

#exec fluxbox | logger -t fluxbox
#EOF
#chmod +x /root/.fluxbox/startup

# awesomewm stuff
mkdir -p ~/.config/awesome
echo "exec /usr/bin/awesome 2>&1 | logger -t awesomewm" > ~/.xinitrc

cat << EOF > /etc/supervisor/conf.d/syslog.conf
[program:syslog]
command=/usr/sbin/syslog-ng -F
priority=1
autostart=true
autorestart=true
stdout_logfile=/var/log/syslog
stderr_logfile=/var/log/syslog
EOF

cat << EOF > /etc/supervisor/conf.d/vnc.conf
[program:vnc]
directory=/opt/tripbot
command=script/x11/start-vnc.sh
autostart=true
autorestart=true
stdout_logfile=syslog
stderr_logfile=syslog
EOF

cat << EOF > /etc/supervisor/conf.d/vlc.conf
[program:vlc]
directory=/opt/tripbot
command=script/x11/start-vlc.sh
autostart=true
autorestart=true
stdout_logfile=syslog
stderr_logfile=syslog
startsecs=2
EOF

# don't autostart OBS if this flag is set
if [ "${DISABLE_OBS}" == "true" ]; then
  echo "Disabling OBS autostart"
  OBS_AUTOSTART="false"
else
  OBS_AUTOSTART="true"
fi

cat << EOF > /etc/supervisor/conf.d/obs.conf
[program:obs]
directory=/opt/tripbot
command=script/x11/start-obs.sh
autostart=$OBS_AUTOSTART
autorestart=true
stdout_logfile=syslog
stderr_logfile=syslog
startsecs=2
EOF

cleanup() {
  echo "Gracefully stopping supervisor"
  supervisorctl stop all
  kill -TERM "$supervisor_pid" 2>/dev/null
  exit 3
}

trap cleanup SIGTERM

nohup supervisord --nodaemon -c /etc/supervisor/supervisord.conf 2>&1 | logger -t supervisor-init &
supervisor_pid=$!

# grc adds color to the output
grc -- tail -F /var/log/syslog
#| grep -v "obs info" | grep -v "obs\ttitle" | grep -v "obs\tclass" | grep -v "obs\tBit depth" | grep -v "obs\tFound proper GLXFBConfig"
