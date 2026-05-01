# OBS Docker container configs

Files baked into the `adanalife/obs` image (`infra/docker/obs/Dockerfile`). They are copied to `/opt/obs/` during build, then staged into `~/.config/obs-studio/` at container startup by `entrypoint.sh`.

## Layout

| File | Role |
|---|---|
| `Dockerfile` | Lives at `infra/docker/obs/Dockerfile` (the image build context); its `COPY` brings everything in this directory into `/opt/obs/`. |
| `entrypoint.sh` | Container `ENTRYPOINT`. Stages configs, runs Xvfb + fluxbox + x11vnc, conditionally writes `service.json` from `STREAM_KEY` via `envsubst`, exec's `obs`. |
| `healthcheck.sh` | Backs the Dockerfile `HEALTHCHECK` — passes when `obs` is running and `xdpyinfo` succeeds against `:0`. |
| `basic.ini` | OBS profile config: 1920×1080@30, simple output, software `obs_x264`. |
| `global.ini` | Bypasses the OBS first-run wizard and points at the `Untitled` profile + `Tripbot` scene collection. |
| `service.json.tmpl` | RTMP service template; `${STREAM_KEY}` is substituted at runtime. Only written when `STREAM_KEY` is set. |
| `Tripbot.json` | Minimal scene collection — one scene `Test` with a color background and a "tripbot stream test" text overlay. |

## Relationship to the host configs in `..`

`configs/obs-studio/{basic.ini,global.ini,service.json,Dashcam_Scenes.*.json}` are the source-of-truth host references — what a developer would drop into `~/.config/obs-studio/` to run OBS natively. The files here are the **docker** variants: trimmed to what works headless under Xvfb, with software encoding so CI doesn't need a GPU.

## Streaming locally

```sh
export STREAM_KEY="<test-channel-key>"
docker compose -f infra/docker/docker-compose.yml build obs
docker compose -f infra/docker/docker-compose.yml up obs
```

Empty / unset `STREAM_KEY` boots the container idle (CI + VNC `localhost:5902`); a real key triggers `--startstreaming`.
