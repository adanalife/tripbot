#!/bin/bash

# this script is the container entrypoint for the OBS container

#TODO: set background in /etc/X11/fluxbox/overlay
#TODO: remove vncconfig from /etc/X11/Xvnc-session

#TODO: remove this?
mkdir -p /opt/data/run

#TODO: remove these
# echo "DANATEST" > /opt/data/run/left-message.txt
# echo "DANATEST" > /opt/data/run/right-message.txt

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

nohup supervisord --nodaemon -c /etc/supervisor/supervisord.conf 2>&1 | logger -t supervisor-init &

tail -F /var/log/syslog
