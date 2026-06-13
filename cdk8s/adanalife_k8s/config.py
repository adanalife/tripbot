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
    obs_encoder: str = "obs_x264"  # ffmpeg_vaapi_tex on GPU envs
    obs_quality: str = "low"  # low | high
    dashcam_mode: str = "hostpath"  # nfs | hostpath
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
    # Streaming platforms present in this env (obs instances). twitch everywhere;
    # youtube currently stage-only while the bot side is built out.
    platforms: tuple[str, ...] = ("twitch",)
    # --- prod-stream protection (2026-06-11 stage-starves-prod incident) ---
    # PriorityClassName stamped on the env's app Deployment pods; when set,
    # SupportingChart also emits the PriorityClass itself. Prod outranks every
    # default-priority (0) pod, so under node pressure the scheduler preempts
    # co-tenant stage workloads, never the live stream.
    priority_class: str = ""
    # CPU requests for the stream-critical pair. Requests are the CFS weight —
    # under CPU contention each cgroup gets CPU proportional to its request, so
    # prod's real-sized requests guarantee the encode/decode chain its share no
    # matter how many 200m co-tenant pods burst. Non-prod stays at the small
    # default so stage/dev keep their light footprint.
    obs_cpu_request: str = "200m"
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
        obs_encoder="ffmpeg_vaapi_tex",
        obs_quality="high",
        dashcam_mode="nfs",
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
        platforms=("twitch",),
        # The live stream always wins: prod app pods outrank default-priority
        # co-tenants (stage, dashcam-cv), and the encode/decode pair carries
        # real CPU requests so contention can't starve it (20-core node; the
        # whole prod chain requests ~3.2 CPU).
        priority_class="prod-stream",
        obs_cpu_request="2",
        vlc_cpu_request="1",
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
        obs_encoder="ffmpeg_vaapi_tex",
        obs_quality="low",
        dashcam_mode="nfs",
        tailscale=True,
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
        # binds chat once a broadcast is live; vlc-youtube self-sustains;
        # obs-youtube boots idle — the streaming toggle is prod-twitch-only).
        # prod follows once the stage burn-in + dual-iGPU-encode validation
        # pass.
        #
        # twitch is OFF here for the duration of the burn-in: running both
        # stage stacks (8 pods, 4 iGPU claims) alongside prod made the prod
        # twitch stream stutter on 2026-06-11 — the stage twitch pair never
        # streamed (no stage stream key exists), but its VLC decode + OBS
        # render still contended for the shared iGPU. Budget is two live
        # streams total: prod-twitch + stage-youtube. Re-add "twitch" when
        # the burn-in ends.
        platforms=("youtube",),
        # Guardrail from the same incident: cap what stage can request in
        # aggregate, so "accidentally scaled up too many stage deployments"
        # parks pods Unschedulable instead of crowding prod off the node.
        # CPU/memory sized roomy — youtube stack (~0.5 CPU / 1.3Gi requests) +
        # dashcam-cv embed jobs (2× 1 CPU / 5Gi) + one-shot jobs fit with
        # headroom; the node has 20 CPU / 31Gi. iGPU claims sized TIGHT to
        # what stage runs today: vlc + obs steady (2) + 1 surge slot for
        # vlc's RollingUpdate maxSurge=1 (obs is Recreate, no surge). Claims,
        # not GPU time — encode contention is governed by the two-stream
        # budget above, not by quota. Bump alongside re-adding twitch.
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
        obs_quality="low",
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
        obs_quality="low",
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
