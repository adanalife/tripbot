"""Tiny admin-shim for the OBS container.

OBS itself doesn't expose an HTTP surface (only obs-websocket on :4455).
The admin panel needs three things from OBS that the other Go services
expose natively via their own HTTP listeners — health, version, and a
shutdown endpoint that triggers a pod restart. This shim provides them
in the same shape so the admin panel can treat OBS like any other
service in the status table.

Flask was picked over stdlib http.server because the route DSL reads
obvious and we already have a Python venv in the image for obsws-python
+ websockify. The traffic shape is one-request-every-few-seconds at
peak (panel render polls), so Flask's built-in development server is
plenty — no need for gunicorn/waitress.
"""

from __future__ import annotations

import os
import signal
import time
from datetime import datetime, timezone
from pathlib import Path

from flask import Flask, jsonify, make_response

# Bake-time version files written by the OBS Dockerfile, matching the
# convention the Go services use (their /version reads VCS info from
# the binary itself; we mirror via files).
VERSION_FILE = Path("/etc/tripbot/version")
SHA_FILE = Path("/etc/tripbot/sha")

# Delay between responding 202 and signalling supervisord, so the
# response reaches the admin panel before the container starts dying.
# Matches the Go services' shutdownDelay.
SHUTDOWN_DELAY_SECONDS = 0.5

# Supervisord runs as PID 1. SIGTERMing PID 1 brings down all the
# supervised programs cleanly and the container exits, which k8s
# restartPolicy: Always then respawns.
SUPERVISORD_PID = 1

app = Flask(__name__)
started_at = time.time()


def _read_or_default(path: Path, default: str) -> str:
    """Best-effort file read — returns default if the file's missing.

    Local docker builds without the /etc/tripbot/* bake step land here.
    """
    try:
        return path.read_text(encoding="utf-8").strip() or default
    except OSError:
        return default


@app.get("/health/ready")
def ready() -> tuple[str, int]:
    """Liveness/readiness — if Flask is answering, the shim is up.

    We deliberately don't poke obs-websocket here: that surface lives
    on :4455 and OBS readiness is its own signal. This shim's job is
    just to expose the admin surface.
    """
    return "OK\n", 200


@app.get("/version")
def version():
    """Build metadata as JSON. Mirrors the Go services' /version shape:
    {tag, sha, built_at, started_at} — the admin panel parses this
    identically across all four services.
    """
    return jsonify(
        tag=_read_or_default(VERSION_FILE, "dev"),
        sha=_read_or_default(SHA_FILE, ""),
        # built_at is the file mtime of /etc/tripbot/version — close
        # enough to "when this image was assembled" for the panel's needs.
        built_at=_built_at(),
        started_at=datetime.fromtimestamp(started_at, tz=timezone.utc).isoformat(),
    )


def _built_at() -> str:
    """Image-build time as RFC3339, or '' if the version file is missing
    (local builds without the bake step)."""
    try:
        mtime = VERSION_FILE.stat().st_mtime
    except OSError:
        return ""
    return datetime.fromtimestamp(mtime, tz=timezone.utc).isoformat()


@app.post("/admin/shutdown")
def shutdown():
    """Trigger a container exit. Schedule SIGTERM-to-supervisord after a
    short delay so the 202 lands on the client first; supervisord's
    shutdown brings down all children, container exits, k8s respawns
    the pod.

    Flask's built-in scheduler is hand-rolled; we use signal.setitimer
    to fire SIGALRM after the delay, then handle it by SIGTERMing pid 1.
    This avoids spawning a threading.Timer in the request path (cleaner
    teardown — the in-flight response finishes, then the timer fires).
    """
    app.logger.warning(
        "admin shutdown requested — SIGTERMing supervisord (pid %d) in %.1fs",
        SUPERVISORD_PID,
        SHUTDOWN_DELAY_SECONDS,
    )
    signal.signal(signal.SIGALRM, _fire_shutdown)
    signal.setitimer(signal.ITIMER_REAL, SHUTDOWN_DELAY_SECONDS)
    return make_response(("shutting down\n", 202))


def _fire_shutdown(_signum, _frame):
    try:
        os.kill(SUPERVISORD_PID, signal.SIGTERM)
    except OSError as exc:  # supervisord already dead, or PID mapping odd
        app.logger.error("failed to SIGTERM supervisord: %s", exc)


def main() -> None:
    """Run the Flask app under supervisord (foreground, no debug/reload).

    Binds to all interfaces inside the pod so the k8s Service (added in
    the matching infra PR) can reach us at obs:8080. Port matches the
    `EXPOSE 8080` directive in the OBS Dockerfile + the supervisor conf.
    """
    app.run(host="0.0.0.0", port=8080, debug=False, use_reloader=False)


if __name__ == "__main__":
    main()
