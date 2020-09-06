#!/bin/bash

# this script is the container entrypoint for the OBS container

cat << EOF > /etc/supervisor/conf.d/syslog.conf
[program:syslog]
command=/usr/sbin/syslog-ng -F
auto_start=true
autorestart=true
stdout_logfile=/var/log/syslog
stderr_logfile=/var/log/syslog
EOF

cat << EOF > /etc/supervisor/conf.d/vnc.conf
[program:vnc]
directory=/opt/tripbot
command=script/x11/start-vnc.sh
auto_start=true
autorestart=true
stdout_logfile=syslog
stderr_logfile=syslog
EOF

cat << EOF > /etc/supervisor/conf.d/vlc.conf
[program:vlc]
directory=/opt/tripbot
command=script/x11/start-vlc.sh
auto_start=true
autorestart=true
stdout_logfile=syslog
stderr_logfile=syslog
EOF

cat << EOF > /etc/supervisor/conf.d/obs.conf
[program:obs]
directory=/opt/tripbot
command=script/x11/start-obs.sh
auto_start=true
autorestart=true
stdout_logfile=syslog
stderr_logfile=syslog
EOF

supervisord --nodaemon -c /etc/supervisor/supervisord.conf
