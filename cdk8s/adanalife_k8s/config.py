"""Per-environment configuration — the matrix knobs the Kustomize overlays vary.

One `EnvConfig` replaces the per-app `overlays/<env>` sprawl. Charts/constructs
read these fields instead of branching on env name inline. App-specific config
that *also* varies by env (the big tripbot literal block) is assembled in
each construct from these knobs; this table holds only the cross-app values.
"""

from __future__ import annotations

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


# The fleet-wide supported-platform set, owned by platform-gateway (its Go
# adapter registry, provider.SupportedPlatforms, is the source of truth) and
# synced into this repo's platforms.json via `task platforms:sync`. Every env's
# `platforms` must be a subset of it (validated at the bottom of this module).
# Never hand-edit platforms.json — add an adapter in the gateway + re-sync.
_PLATFORMS_FILE = Path(__file__).resolve().parents[2] / "platforms.json"


def _load_supported_platforms() -> tuple[str, ...]:
    import json

    with _PLATFORMS_FILE.open() as f:
        return tuple(json.load(f)["platforms"])


SUPPORTED_PLATFORMS = _load_supported_platforms()


@dataclass(frozen=True)
class EnvConfig:
    name: str  # prod-1 | stage-1 | development | local
    namespace: str
    cluster: str  # minipc | k3d | local
    aws_account: str  # adanalife-prod | adanalife-stage | "" (local)
    image_tag: str  # floating tag (latest | main) for components without a pin
    dns_base: str  # prod.whereisdana.today | stage... | dev...  ("" for local)
    nats_url: str
    sentry_env: str  # SENTRY_ENVIRONMENT (prod-1 | stage-1 | development)
    # Per-component pinned release tags, loaded from versions.yaml by load_env.
    # Keyed by image name (tripbot, obs, onscreens-server). Pinned
    # components deploy that exact tag with IfNotPresent (release tags are
    # immutable); unpinned ones fall back to image_tag with Always.
    image_pins: dict[str, str] = field(default_factory=dict)
    binary_env: str = "development"  # ENV= the Go config validator accepts: production|staging|development
    deployment_env: str = (
        "development"  # OTEL deployment.environment + telemetry env id
    )
    secret_source: str = "eso"  # eso | local
    gpu: bool = False  # request gpu.intel.com/i915
    tailscale: bool = False  # emit the tailscale Ingress
    otel: bool = False  # OTEL_SDK_DISABLED=false when True
    postgres_size: str = "5Gi"
    postgres_storage_class: str = ""  # "" = cluster default; local-path-retain on prod
    postgres_backup: bool = False
    # "" → postgres co-locates in the app namespace (default, byte-identical
    # render). Set to an isolated namespace (e.g. "stage-1-data") to move the DB
    # StatefulSet + its ESO SecretStore out of the app namespace, so deleting the
    # app namespace can't drop the database. Apps reach it cross-namespace via the
    # postgres_host FQDN.
    data_namespace: str = ""
    external_dns_role_arn: str = (
        ""  # cert-manager DNS-01 Route53 role (per AWS account)
    )
    lan_ip: str = (
        "192.168.1.200"  # mini-PC node IP external-dns/traefik target (platform Helm)
    )
    # Streaming platforms present in this env. twitch everywhere; youtube
    # currently stage-only while the bot side is built out. Drives the per-platform
    # fan-out of tripbot/onscreens (OBS itself is deployed by the obs repo now,
    # which carries its own obs_streaming for the stream-key + --startstreaming).
    platforms: tuple[str, ...] = ("twitch",)
    # --- prod-stream protection (2026-06-11 stage-starves-prod incident) ---
    # PriorityClassName stamped on the env's app Deployment pods; when set,
    # SupportingChart also emits the PriorityClass itself. Prod outranks every
    # default-priority (0) pod, so under node pressure the scheduler preempts
    # co-tenant stage workloads, never the live stream.
    priority_class: str = ""
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
    # Bias this env's stateless app pods toward the ephemeral arm64 rpi5 worker
    # (adanalife-rpi5) when it's present, falling back to the MS-01 when it's not.
    # When True, the tripbot/onscreens constructs add a toleration for the
    # node's dana.lol/rpi5 taint + a PREFERRED (never required) node affinity
    # toward dana.lol/board=rpi5 (see scheduling.py). onscreens follows the OBS
    # pod (colocate_with_obs_affinity) rather than carrying the board affinity
    # itself. Stage only; prod stays on the MS-01 (and the taint repels it
    # regardless, since prod pods carry no toleration).
    prefer_rpi5: bool = False

    # The platform-gateway gateway-twitch URL a twitch instance routes its
    # Helix calls through — the gateway is tripbot's sole Helix caller. Empty
    # leaves the gateway unwired (local/CI only): the Twitch audience/follower/
    # broadcaster-send features are disabled.
    twitch_api_url: str = ""
    # Like twitch_api_url, but for a youtube instance's outbound chat sends:
    # gateway-youtube's URL routes them through the platform-gateway
    # unconditionally (no runtime flag — unlike Twitch). Empty keeps the
    # in-process pkg/youtube send. The inbound chat poll stays in-process
    # regardless (no gateway streaming endpoint).
    youtube_api_url: str = ""
    # The gateway-transport platforms: a PLATFORM=facebook/instagram/tiktok
    # instance reaches BOTH chat directions through its per-platform gateway
    # instance (inbound poll always; outbound sends where the platform is
    # two-way). Required on those instances — with the URL empty the pod
    # boots without platform chat.
    facebook_api_url: str = ""
    instagram_api_url: str = ""
    tiktok_api_url: str = ""
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
        no silent drift). Floating tags (latest/main) need Always to pick up
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
        tailscale=True,
        otel=True,
        postgres_size="50Gi",
        postgres_storage_class="local-path-retain",
        postgres_backup=True,
        external_dns_role_arn=_PROD_ROLE,
        # The DB lives in its own namespace so a `kubectl delete ns prod-1` can't
        # take years of irreplaceable data.
        data_namespace="prod-1-data",
        # Every platform's app stack (tripbot / onscreens) is emitted and Argo
        # manages it, but births parked at replicas:0 — a console scale-up brings
        # one live (Argo ignores .spec.replicas, so the scale sticks). Only twitch
        # runs today; youtube waits on the pending Data API quota extension
        # (youtube_inbound_enabled below), and facebook streams to the real ADL
        # Page once scaled up (gateway-facebook holds the Page token). Manifests
        # render while parked — e.g. the tripbot-youtube-creds ExternalSecret
        # (prod-account SM k8s/tripbot/youtube-creds) keeps syncing, which
        # gateway-youtube also relies on.
        platforms=("twitch", "youtube", "facebook"),
        # prod youtube launches bot-less: inbound chat poll off (quota extension
        # pending), so rotators serve promo copy and no command responds. Flip to
        # True when the YouTube Data API quota lands. See youtube_inbound_enabled.
        youtube_inbound_enabled=False,
        # Route prod tripbot-youtube's outbound chat sends through the in-namespace
        # gateway-youtube (the gateway owns the YouTube token). Mirrors stage. The
        # prod gateway holds a YouTube token as of 2026-06-22, so this is safe to
        # ship; without a gateway token, sends would fail.
        youtube_api_url="http://gateway-youtube.prod-1.svc.cluster.local:8080",
        # Route prod tripbot-facebook's chat sends through the in-namespace
        # gateway-facebook (the gateway owns the Page token). Mirrors stage.
        facebook_api_url="http://gateway-facebook.prod-1.svc.cluster.local:8080",
        # Wire prod tripbot-twitch to gateway-twitch (in-namespace). Required:
        # since the cutover the gateway is the unconditional single Helix caller
        # (the twitch_gateway flag and the in-process fallback are gone).
        twitch_api_url="http://gateway-twitch.prod-1.svc.cluster.local:8080",
        # The live stream always wins: prod app pods outrank default-priority
        # co-tenants (stage, dashcam-cv) under node pressure. The playback
        # decode/encode CPU requests live in the playout and obs repos now.
        priority_class="prod-stream",
    ),
    "stage-1": EnvConfig(
        name="stage-1",
        namespace="stage-1",
        cluster="minipc",
        aws_account="adanalife-stage",
        image_tag="main",
        dns_base="stage.whereisdana.today",
        nats_url="nats://nats.stage-1-platform.svc.cluster.local:4222",
        sentry_env="stage-1",
        binary_env="staging",
        deployment_env="stage-1",
        gpu=True,
        tailscale=True,
        # Prefer the ephemeral arm64 rpi5 worker for stage's stateless app pods
        # (tripbot/onscreens); they recover onto the MS-01 if the Pi is
        # unplugged. See prefer_rpi5 on EnvConfig + scheduling.py.
        prefer_rpi5=True,
        otel=False,
        postgres_size="10Gi",
        postgres_storage_class="local-path",
        external_dns_role_arn=_STAGE_ROLE,
        # Stage rehearses DB-in-its-own-namespace: postgres + its SecretStore land
        # in stage-1-data, so a `kubectl delete ns stage-1` can't take the DB. prod
        # follows on its next wipe (set prod-1's data_namespace to prod-1-data).
        data_namespace="stage-1-data",
        # Every stage platform is present but births parked at replicas:0 — the
        # resting state is everything-off, and a platform comes online via the
        # console's scale-up button (Argo ignores .spec.replicas, so the hand
        # scale sticks). facebook is the current burn-in platform, chatting
        # against the ADL Staging Page.
        #
        # Extra stage stream workloads contending for the shared node is what
        # stutters the prod stream — budget is two live streams total:
        # prod-twitch + one stage burn-in.
        platforms=("youtube", "twitch", "tiktok", "facebook", "instagram"),
        # Route stage tripbot-twitch's Helix calls through the in-namespace
        # gateway-twitch.
        twitch_api_url="http://gateway-twitch.stage-1.svc.cluster.local:8080",
        # Route stage tripbot-youtube's outbound chat sends through the
        # in-namespace gateway-youtube (unconditionally — no flag). The inbound
        # poll stays in-process.
        youtube_api_url="http://gateway-youtube.stage-1.svc.cluster.local:8080",
        # The parked platform instances point at their in-namespace gateway
        # the same way, so a hand scale-up is a working bring-up rather than
        # a chat-less pod waiting on a config edit.
        facebook_api_url="http://gateway-facebook.stage-1.svc.cluster.local:8080",
        instagram_api_url="http://gateway-instagram.stage-1.svc.cluster.local:8080",
        tiktok_api_url="http://gateway-tiktok.stage-1.svc.cluster.local:8080",
        # Guardrail from the same incident: cap what stage can request in
        # aggregate, so "accidentally scaled up too many stage deployments"
        # parks pods Unschedulable instead of crowding prod off the node.
        # CPU/memory sized roomy — youtube stack (~0.5 CPU / 1.3Gi requests) +
        # dashcam-cv embed jobs (2× 1 CPU / 5Gi) + one-shot jobs fit with
        # headroom; the node has 20 CPU / 31Gi. iGPU cap of 3 covers stage
        # obs-youtube's own claim (1, re-enabled 2026-06-19) plus the
        # video-optimization job's claim with surge headroom.
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
        image_tag="main",
        dns_base="dev.whereisdana.today",
        nats_url="nats://nats.development-platform.svc.cluster.local:4222",
        sentry_env="development",
        binary_env="staging",
        deployment_env="development",
        gpu=False,
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
        tailscale=False,
        otel=False,
        platforms=("twitch",),
    ),
}


# Guard: an env can only run platforms the gateway has an adapter for. A
# platform with no adapter has no chat transport, so reject it at synth time
# rather than emitting a dead instance.
for _name, _env in ENVS.items():
    _unknown = tuple(p for p in _env.platforms if p not in SUPPORTED_PLATFORMS)
    if _unknown:
        raise ValueError(
            f"{_name}: platforms {_unknown} not in SUPPORTED_PLATFORMS "
            f"{SUPPORTED_PLATFORMS} — add an adapter in platform-gateway + run `task platforms:sync`"
        )


def load_env(name: str) -> EnvConfig:
    try:
        env = ENVS[name]
    except KeyError:
        raise SystemExit(f"unknown env {name!r}; known: {', '.join(ENVS)}")
    from dataclasses import replace

    # Per-component release pins ride in from versions.yaml rather than the
    # static table above, so release automation edits one data file.
    pins = image_pins().get(name)
    if pins:
        env = replace(env, image_pins=dict(pins))
    return env
