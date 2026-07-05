"""Per-environment configuration — the matrix knobs the Kustomize overlays vary.

One `EnvConfig` replaces the per-app `overlays/<env>` sprawl. Charts/constructs
read these fields instead of branching on env name inline. App-specific config
that *also* varies by env (the big vlc/tripbot literal blocks) is assembled in
each construct from these knobs; this table holds only the cross-app values.
"""

from __future__ import annotations

import os
from dataclasses import dataclass, field
from functools import lru_cache
from pathlib import Path

import yaml

# Per-component image pins (cdk8s/versions.yaml). Envs present in the file
# deploy pinned release tags; the rest float on EnvConfig.image_tag.
_VERSIONS_FILE = Path(__file__).resolve().parents[1] / "versions.yaml"


@lru_cache(maxsize=1)
def image_pins() -> dict[str, dict[str, str]]:
    with _VERSIONS_FILE.open() as f:
        return yaml.safe_load(f) or {}


@dataclass(frozen=True)
class EnvConfig:
    name: str  # prod-1 | stage-1 | development | local
    namespace: str
    cluster: str  # minipc | k3d | local
    aws_account: str  # adanalife-prod | adanalife-stage | "" (local)
    image_tag: str  # floating tag (latest | develop) for components without a pin
    dns_base: str  # prod.whereisdana.today | stage... | dev...  ("" for local)
    nats_url: str
    sentry_env: str  # SENTRY_ENVIRONMENT (prod-1 | stage-1 | development)
    # Per-component pinned release tags, loaded from versions.yaml by load_env.
    # Keyed by image name (tripbot, vlc, obs, onscreens-server). Pinned
    # components deploy that exact tag with IfNotPresent (release tags are
    # immutable); unpinned ones fall back to image_tag with Always.
    image_pins: dict[str, str] = field(default_factory=dict)
    binary_env: str = "development"  # ENV= the Go config validator accepts: production|staging|development
    deployment_env: str = (
        "development"  # OTEL deployment.environment + telemetry env id
    )
    secret_source: str = "eso"  # eso | local
    gpu: bool = False  # request gpu.intel.com/i915
    # vlc-server's iGPU claim is gated on (gpu and vlc_gpu). Default True keeps the
    # claim wherever the env has a GPU; set False to drop just vlc's claim while OBS
    # keeps the iGPU. vlc proved it doesn't need the GPU (stream-copy + trivial
    # software decode — CPU flat at ~0.04 cores with and without /dev/dri,
    # verified live on stage 2026-06-13). See VlcServer in constructs/vlc.py.
    vlc_gpu: bool = True
    dashcam_mode: str = "hostpath"  # nfs | hostpath
    # Which PVC vlc mounts the corpus from when dashcam_mode == "nfs": the
    # NFS-backed `vlc-dashcam` (default) or the node-local `vlc-dashcam-local`
    # cache. The local PVC + its NFS->local copy Job are provisioned by infra's
    # dashcam_local_enabled flag; this only picks which claim vlc mounts. Flip back
    # to "nfs" for an instant fallback while the local copy is (re)populated.
    dashcam_source: str = "nfs"  # nfs | local (only meaningful when dashcam_mode=nfs)
    tailscale: bool = False  # emit the tailscale Ingress
    otel: bool = False  # OTEL_SDK_DISABLED=false when True
    postgres_size: str = "5Gi"
    postgres_storage_class: str = ""  # "" = cluster default; local-path-retain on prod
    postgres_backup: bool = False
    # "" → postgres co-locates in the app namespace (default, byte-identical
    # render). Set to an isolated namespace (e.g. "stage-1-data") to move the DB
    # StatefulSet + its ESO SecretStore out of the app namespace, so deleting the
    # app namespace can't drop the database. Apps reach it cross-namespace via the
    # postgres_host FQDN. The dashcam PVC does NOT move (vlc mounts it; PVCs are
    # namespace-local) — it shifts to SupportingChart in the app namespace.
    data_namespace: str = ""
    external_dns_role_arn: str = (
        ""  # cert-manager DNS-01 Route53 role (per AWS account)
    )
    lan_ip: str = (
        "192.168.1.200"  # mini-PC node IP external-dns/traefik target (platform Helm)
    )
    nfs_server: str = ""  # dashcam NFS export (nfs mode); from $NFS_SERVER at synth
    nfs_path: str = ""  # dashcam NFS path; from $NFS_PATH at synth
    nfs_pv_name: str = (
        "vlc-dashcam-nfs"  # PVs bind 1:1 — stage needs its own (vlc-dashcam-nfs-stage)
    )
    # Streaming platforms present in this env. twitch everywhere; youtube
    # currently stage-only while the bot side is built out. Drives the per-platform
    # fan-out of tripbot/vlc/onscreens (OBS itself is deployed by the obs repo now,
    # which carries its own obs_streaming for the stream-key + --startstreaming).
    platforms: tuple[str, ...] = ("twitch",)
    # --- prod-stream protection (2026-06-11 stage-starves-prod incident) ---
    # PriorityClassName stamped on the env's app Deployment pods; when set,
    # SupportingChart also emits the PriorityClass itself. Prod outranks every
    # default-priority (0) pod, so under node pressure the scheduler preempts
    # co-tenant stage workloads, never the live stream.
    priority_class: str = ""
    # CPU request for vlc-server (the stream-critical decode side; OBS, the encode
    # side, is sized by the obs repo now). Requests are the CFS weight — under CPU
    # contention each cgroup gets CPU proportional to its request, so prod's
    # real-sized request guarantees the decode chain its share no matter how many
    # 200m co-tenant pods burst. Non-prod stays at the small default so stage/dev
    # keep their light footprint.
    vlc_cpu_request: str = "200m"
    # ResourceQuota hard caps for the app namespace (emitted by SupportingChart
    # when non-empty). Caps what the env can REQUEST in aggregate — scaling up
    # too many deployments hits the quota and pods stay unscheduled instead of
    # crowding the node. NB: quota on requests.* means every pod in the
    # namespace must declare requests for those resources or be rejected.
    app_quota: dict[str, str] = field(default_factory=dict)
    # Non-standard public HTTPS port carried in externally-visible URLs
    # (EXTERNAL_URL, registered OAuth redirect URIs). Only dev needs it — k3d's
    # traefik is mapped to host :9443 because Colima can't bind :443.
    external_port: str = ""
    # Fixed NodePort exposing vlc-twitch's RTSP listener on the node IP, so a LAN
    # box (e.g. OBS on a desktop) can pull rtsp://<node-ip>:<port>/dashcam without
    # kubectl/port-forward. 0 = no NodePort (default). Only the twitch instance
    # emits it. The minipc has no LoadBalancer controller, so this is the stable-
    # host-endpoint equivalent of the k3d-only `<name>-host` LoadBalancer. prod-1
    # and stage-1 co-tenant the one minipc node, and a pinned NodePort can't be
    # claimed twice on the same node — each env that wants one must pick a distinct
    # number in the 30000-32767 range. RTSP only; VNC/HTTP stay in-cluster.
    vlc_rtsp_node_port: int = 0
    # Bias this env's stateless app pods toward the ephemeral arm64 rpi5 worker
    # (adanalife-rpi5) when it's present, falling back to the MS-01 when it's not.
    # When True, the tripbot/vlc/onscreens constructs add a toleration for the
    # node's dana.lol/rpi5 taint + a PREFERRED (never required) node affinity
    # toward dana.lol/board=rpi5 (see scheduling.py). vlc/onscreens follow the OBS
    # pod (colocate_with_obs_affinity) rather than carrying the board affinity
    # themselves. Stage only; prod stays on the MS-01 (and the taint repels it
    # regardless, since prod pods carry no toleration).
    prefer_rpi5: bool = False
    # When True, the app Deployments omit spec.replicas, so Argo never manages
    # the replica count and a hand `kubectl scale` / console start-stop button is
    # authoritative (survives autosync). Stage only — it's where components are
    # parked at 0 to keep the minipc free for prod + the shared transcode job.
    # Pairs with stage selfHeal being off in the Argo apps set (infra
    # SELFHEAL_OFF_ENVS). prod keeps replicas declared so Argo holds it at 1.
    manual_replicas: bool = False
    # Subset of `platforms` whose app Deployments render with spec.replicas=0 —
    # the whole platform stack (tripbot/vlc/onscreens) is emitted and Argo
    # manages it, but parked off so it consumes no node resources until turned
    # on by removing the platform from this set (a config edit + redeploy; under
    # prod selfHeal a hand `kubectl scale` would just be reverted to 0). Lets a
    # platform be staged on an env ahead of being made live. prod-youtube sits
    # here until stage-youtube is shut down and prod-youtube is turned on, so the
    # minipc never runs two youtube stacks at once.
    parked_platforms: tuple[str, ...] = ()

    @property
    def replicas(self) -> int | None:
        """spec.replicas for the app Deployments: None (omitted, manually
        scaled) when manual_replicas, else 1."""
        return None if self.manual_replicas else 1

    def replicas_for(self, platform: str) -> int | None:
        """spec.replicas for a given platform's app Deployments: 0 when the
        platform is parked (rendered but off), else the env-wide `replicas`."""
        return 0 if platform in self.parked_platforms else self.replicas

    # The platform-gateway gateway-twitch URL the chatbot routes its command-time
    # Helix calls through (Phase 3). Empty keeps tripbot's in-process pkg/twitch
    # path; set per env to flip App.Twitch to the HTTP client. Stage points at
    # its in-namespace gateway-twitch Service; prod stays in-process until the
    # gateway's prod release is cut + proven.
    twitch_api_url: str = ""
    # Like twitch_api_url, but for a youtube instance's outbound chat sends:
    # gateway-youtube's URL routes them through the platform-gateway
    # unconditionally (no runtime flag — unlike Twitch). Empty keeps the
    # in-process pkg/youtube send. The inbound chat poll stays in-process
    # regardless (no gateway streaming endpoint).
    youtube_api_url: str = ""
    # Gate the youtube instance's inbound chat poll (tripbot's
    # YOUTUBE_INBOUND_ENABLED + onscreens' rotator copy). False = bot-less
    # YouTube: outbound rotators + background jobs run, but nothing reads chat
    # (no command responds) and the rotators advertise promo copy pointing at
    # Twitch instead of commands. Prod launches bot-less while the YouTube Data
    # API quota extension is pending — prod's 2s poll floor would blow the
    # default 10k/day quota. Flip to True the day the extension lands. Stage
    # stays True: its 10s floor fits the default quota, so it runs the full bot
    # for testing. Only meaningful on a youtube instance.
    youtube_inbound_enabled: bool = True

    def tag_for(self, component: str) -> str:
        """Image tag for a component: its pinned release tag when versions.yaml
        pins it for this env, else the env's floating tag."""
        return self.image_pins.get(component, self.image_tag)

    def pull_policy_for(self, component: str) -> str:
        """Pinned release tags are immutable → IfNotPresent (no redundant pulls,
        no silent drift). Floating tags (latest/develop) need Always to pick up
        rebuilds under the same tag."""
        return "IfNotPresent" if component in self.image_pins else "Always"

    @property
    def otel_disabled(self) -> str:
        """OTEL_SDK_DISABLED literal — disabled everywhere OTEL isn't on."""
        return "false" if self.otel else "true"

    @property
    def tls(self) -> bool:
        """Whether app ingresses get cert-manager TLS (minipc envs only)."""
        return self.cluster == "minipc"

    @property
    def data_ns(self) -> str:
        """Namespace the stateful data unit (postgres + its SecretStore) lands in:
        the app namespace by default, or the isolated one when data_namespace set."""
        return self.data_namespace or self.namespace

    @property
    def data_isolated(self) -> bool:
        """True when postgres lives in its own namespace, split from the app ns."""
        return bool(self.data_namespace) and self.data_namespace != self.namespace

    @property
    def postgres_host(self) -> str:
        """DATABASE_HOST apps connect to: the bare Service name when co-located
        (parity), the cross-namespace FQDN when the DB is isolated."""
        return (
            f"postgres.{self.data_namespace}.svc.cluster.local"
            if self.data_isolated
            else "postgres"
        )


# Stage and dev share the adanalife-stage account → same ExternalDNSRole ARN.
_STAGE_ROLE = "arn:aws:iam::413585268653:role/ExternalDNSRole"
_PROD_ROLE = "arn:aws:iam::704461573429:role/ExternalDNSRole"


# Per-env table. Mirrors the Kustomize overlays; the source of truth once those
# overlays are retired. Values cross-checked against k8s/apps/*/overlays/<env>.
ENVS: dict[str, EnvConfig] = {
    "prod-1": EnvConfig(
        name="prod-1",
        namespace="prod-1",
        cluster="minipc",
        aws_account="adanalife-prod",
        image_tag="latest",
        dns_base="prod.whereisdana.today",
        nats_url="nats://nats.prod-1-platform.svc.cluster.local:4222",
        sentry_env="prod-1",
        binary_env="production",
        deployment_env="prod-1",
        gpu=True,
        # vlc doesn't need the iGPU (stream-copy + software decode); dropping its
        # claim frees an iGPU slot and eases co-tenant contention (the 2026-06-11
        # prod-stutter incident). OBS keeps the iGPU for VAAPI encode (sized in the
        # obs repo now). Proven on stage first, then prod on the live twitch stream.
        vlc_gpu=False,
        dashcam_mode="nfs",
        dashcam_source="local",  # serve the corpus off the minipc's local NVMe copy
        tailscale=True,
        otel=True,
        postgres_size="50Gi",
        postgres_storage_class="local-path-retain",
        postgres_backup=True,
        external_dns_role_arn=_PROD_ROLE,
        nfs_pv_name="vlc-dashcam-nfs",
        # The DB lives in its own namespace so a `kubectl delete ns prod-1` can't
        # take years of irreplaceable data.
        data_namespace="prod-1-data",
        # youtube is LIVE (unparked) — the prod-youtube app stack (tripbot /
        # vlc / onscreens) runs at replicas=1, streaming unlisted to burn in
        # before the public launch. stage-youtube is scaled down first so the
        # minipc never runs two youtube encoders at once. The youtube tripbot
        # instance pulls tripbot-youtube-creds from prod-account SM
        # k8s/tripbot/youtube-creds — that must be seeded for its ExternalSecret
        # to sync.
        platforms=("twitch", "youtube"),
        # prod youtube launches bot-less: inbound chat poll off (quota extension
        # pending), so rotators serve promo copy and no command responds. Flip to
        # True when the YouTube Data API quota lands. See youtube_inbound_enabled.
        youtube_inbound_enabled=False,
        # Route prod tripbot-youtube's outbound chat sends through the in-namespace
        # gateway-youtube (the gateway owns the YouTube token). Mirrors stage. The
        # prod gateway holds a YouTube token as of 2026-06-22, so this is safe to
        # ship; without a gateway token, sends would fail.
        youtube_api_url="http://gateway-youtube.prod-1.svc.cluster.local:8080",
        # Wire prod tripbot-twitch to gateway-twitch (in-namespace). Required:
        # since the cutover the gateway is the unconditional single Helix caller
        # (the twitch_gateway flag and the in-process fallback are gone).
        twitch_api_url="http://gateway-twitch.prod-1.svc.cluster.local:8080",
        # The live stream always wins: prod app pods outrank default-priority
        # co-tenants (stage, dashcam-cv), and vlc's decode side carries a real CPU
        # request so contention can't starve it (20-core node). OBS's matching
        # encode-side request lives in the obs repo now.
        priority_class="prod-stream",
        vlc_cpu_request="1",
        # Stable LAN endpoint for pulling the dashcam RTSP feed off-cluster
        # (e.g. OBS on a desktop) without kubectl: rtsp://<minipc-ip>:30854/dashcam
        # (TCP transport). Distinct from any future stage NodePort (same node).
        vlc_rtsp_node_port=30854,
    ),
    "stage-1": EnvConfig(
        name="stage-1",
        namespace="stage-1",
        cluster="minipc",
        aws_account="adanalife-stage",
        image_tag="develop",
        dns_base="stage.whereisdana.today",
        nats_url="nats://nats.stage-1-platform.svc.cluster.local:4222",
        sentry_env="stage-1",
        binary_env="staging",
        deployment_env="stage-1",
        gpu=True,
        # vlc's iGPU claim proven unnecessary 2026-06-13 (stream-copy + trivial
        # software decode). Re-asserting vlc_gpu=False here — it was lost when the
        # app workloads moved from infra into this repo (the cdk8s-into-repo
        # cutover dropped the flag, so stage vlc silently reclaimed the iGPU).
        vlc_gpu=False,
        dashcam_mode="nfs",
        tailscale=True,
        # Prefer the ephemeral arm64 rpi5 worker for stage's stateless app pods
        # (tripbot/vlc/onscreens); they recover onto the MS-01 if the Pi is
        # unplugged. See prefer_rpi5 on EnvConfig + scheduling.py.
        prefer_rpi5=True,
        otel=False,
        postgres_size="10Gi",
        postgres_storage_class="local-path",
        external_dns_role_arn=_STAGE_ROLE,
        nfs_pv_name="vlc-dashcam-nfs-stage",
        # Stage rehearses DB-in-its-own-namespace: postgres + its SecretStore land
        # in stage-1-data, so a `kubectl delete ns stage-1` can't take the DB. prod
        # follows on its next wipe (set prod-1's data_namespace to prod-1-data).
        data_namespace="stage-1-data",
        # The YouTube platform stack burns in on stage first (tripbot-youtube
        # binds chat once a broadcast is live; vlc-youtube self-sustains). prod
        # follows once the stage burn-in + dual-iGPU-encode validation pass.
        #
        # twitch is back ON (2026-06-19) to test the platform-gateway end to
        # end: stage tripbot-twitch routes its Helix calls through gateway-twitch
        # (twitch_api_url below). The 2026-06-11 prod-stutter that forced twitch
        # OFF here was the stage twitch *VLC decode + OBS render* contending for
        # the shared iGPU — so only tripbot-twitch (no GPU) is meant to run;
        # vlc/onscreens-twitch stay scaled to 0 (manual_replicas below + stage
        # selfHeal off, so a hand/console scale sticks). Budget is still two live
        # streams total: prod-twitch + stage-youtube.
        platforms=("youtube", "twitch"),
        # Stage components are scaled up/down by hand (only tripbot-twitch runs
        # for the gateway test); omit replicas so Argo doesn't reset them.
        manual_replicas=True,
        # Route stage tripbot's command-time Helix calls through the in-namespace
        # gateway-twitch gateway (Phase 3). prod stays in-process until its gateway
        # release is cut.
        twitch_api_url="http://gateway-twitch.stage-1.svc.cluster.local:8080",
        # Route stage tripbot-youtube's outbound chat sends through the
        # in-namespace gateway-youtube (unconditionally — no flag). The inbound
        # poll stays in-process. prod has no youtube instance yet.
        youtube_api_url="http://gateway-youtube.stage-1.svc.cluster.local:8080",
        # Guardrail from the same incident: cap what stage can request in
        # aggregate, so "accidentally scaled up too many stage deployments"
        # parks pods Unschedulable instead of crowding prod off the node.
        # CPU/memory sized roomy — youtube stack (~0.5 CPU / 1.3Gi requests) +
        # dashcam-cv embed jobs (2× 1 CPU / 5Gi) + one-shot jobs fit with
        # headroom; the node has 20 CPU / 31Gi. iGPU cap of 3 covers stage
        # obs-youtube's own claim (1, re-enabled 2026-06-19) plus the
        # video-optimization job's claim with surge headroom; vlc_gpu stays False
        # (stream-copy needs no device).
        app_quota={
            "requests.cpu": "6",
            "requests.memory": "16Gi",
            "requests.gpu.intel.com/i915": "3",
            "pods": "30",
        },
    ),
    "development": EnvConfig(
        name="development",
        namespace="development",
        cluster="k3d",
        aws_account="adanalife-stage",
        image_tag="develop",
        dns_base="dev.whereisdana.today",
        nats_url="nats://nats.development-platform.svc.cluster.local:4222",
        sentry_env="development",
        binary_env="staging",
        deployment_env="development",
        gpu=False,
        dashcam_mode="hostpath",
        tailscale=False,
        otel=False,
        external_dns_role_arn=_STAGE_ROLE,
        platforms=("twitch",),
        external_port="9443",
    ),
    "local": EnvConfig(
        name="local",
        namespace="default",
        cluster="local",
        aws_account="",
        image_tag="latest",
        dns_base="",
        nats_url="",
        sentry_env="development",
        binary_env="staging",
        deployment_env="development",
        secret_source="local",
        gpu=False,
        dashcam_mode="hostpath",
        tailscale=False,
        otel=False,
        platforms=("twitch",),
    ),
}


def load_env(name: str) -> EnvConfig:
    try:
        env = ENVS[name]
    except KeyError:
        raise SystemExit(f"unknown env {name!r}; known: {', '.join(ENVS)}")
    from dataclasses import replace

    # Per-component release pins ride in from versions.yaml rather than the
    # static table above, so the bump-prs workflow edits one data file.
    pins = image_pins().get(name)
    if pins:
        env = replace(env, image_pins=dict(pins))
    # NFS coordinates are deployment-host-specific (gitignored in Kustomize as
    # dashcam-nfs.local.yaml); thread them in from the environment at synth so
    # they never get committed. Placeholders match the legacy .example render.
    if env.dashcam_mode == "nfs":
        env = replace(
            env,
            nfs_server=os.environ.get("NFS_SERVER", "<NFS server address>"),
            nfs_path=os.environ.get("NFS_PATH", "<export path>"),
        )
    return env
