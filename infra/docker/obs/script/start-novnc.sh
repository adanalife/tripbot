#!/usr/bin/env bash
# Supervisor program: serve the noVNC browser client on :6080, bridging the
# browser's WebSocket to wayvnc's TCP :5900 via websockify.
#
# The obs Ingress (traefik) terminates TLS and forwards plain HTTP here, so
# websockify itself stays unencrypted in-pod. This is the access path that
# replaces port-forwarding a native VNC client — and it works with macOS
# Screen Sharing.app's blind spot, because noVNC speaks standard RFB rather
# than Apple's RFB 003.889 (which neatvnc rejects at the version handshake,
# before any security type is negotiated).
set -euo pipefail

# Block until wayvnc is accepting on :5900 — websockify's target must be live
# or the browser connects to a dead proxy. wayvnc itself waits on the sway
# Wayland socket, so this transitively waits for the whole display stack.
for _ in $(seq 1 60); do
  if (exec 3<>/dev/tcp/127.0.0.1/5900) 2>/dev/null; then
    exec 3>&- 3<&-
    break
  fi
  sleep 0.5
done

if ! (exec 3<>/dev/tcp/127.0.0.1/5900) 2>/dev/null; then
  echo "novnc: wayvnc never came up on :5900" >&2
  exit 1
fi
exec 3>&- 3<&-

# --web serves noVNC's static assets; the same listener upgrades /websockify
# to the WebSocket→TCP bridge. vnc.html is symlinked to index.html in the
# image, so the Ingress root lands on the client directly.
exec /opt/obs/venv/bin/websockify --web /opt/novnc 6080 127.0.0.1:5900
