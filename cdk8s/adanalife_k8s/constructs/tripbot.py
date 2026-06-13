"""Tripbot — the chatbot Deployment + Service + Ingress + its ExternalSecrets,
plus the one-shot bootstrap/seed Jobs as module-level emitters.

Reproduces k8s/apps/tripbot/base + overlays:

  * Deployment: a `migrate` initContainer (migrate-to-head before boot) + the
    `tripbot` container, both PodSecurity-restricted (runAsNonRoot 65532,
    seccomp RuntimeDefault, drop-ALL). USER=tripbot so OTel's process resource
    detector doesn't crash the SDK on a no-/etc/passwd static binary.
  * envFrom order is load-bearing — config first, then DB creds, then the
    shared OTLP/Sentry Secrets, then twitch/maps (required) and the two
    optional discord Secrets. On the laptop the DB Secret is the on-disk
    `tripbot-secret`; on eso envs it's the ESO `tripbot-database-creds`.
  * Service (ClusterIP :8080) + traefik Ingress everywhere (the web UI / OAuth
    round-trip is reachable on-LAN in every env). The *bot* is outbound-only
    (EventSub via WebSocket), but the dashboard Ingress is published per env;
    minipc envs add TLS + a Tailscale Ingress. local is HTTP-only at
    tripbot.localhost.
The construct envFroms its DB + app Secrets by name but does NOT emit them —
they're identity-level (one bot, one DB, shared by every platform stack), so
`emit_identity_secrets` emits them once into the per-env supporting unit
(SupportingChart): database (ESO target.template remap, or the on-disk
`tripbot-secret` on the laptop), twitch + google-maps (extract), discord-alerts
+ discord-bot-token (bare remoteRef). The one exception is the youtube
instance's `tripbot-youtube-creds` (YouTube OAuth client) — per-platform, not
identity-level, so the construct emits that ExternalSecret itself.

The `tripbot-config` ConfigMap keeps its STABLE name (not the kustomize hash)
so the one-shot Jobs below can `envFrom` it; the pod template still rolls on
config change via the `adanalife.dev/config-hash` annotation (configmap.py).
The Jobs are NOT auto-emitted by the construct — they're module-level functions
the deploy tasks call on demand, so a routine `apply` never runs a one-shot.
"""

from __future__ import annotations

import cdk8s
from constructs import Construct

import imports.k8s as k8s
from adanalife_k8s import appconfig, configmap, eso
from adanalife_k8s.config import EnvConfig
from adanalife_k8s.eso import ESData
from adanalife_k8s.naming import app_name, meta_labels, selector

IMAGE = "adanalife/tripbot"
DB_SECRET_NAME = "tripbot-database-creds"  # ESO-materialized DB creds (eso envs)
LOCAL_DB_SECRET = "tripbot-secret"  # secret.env-built DB creds (laptop)
# Identity label for the shared (non-per-platform) Secrets — they belong to the
# bot identity, not any one platform stack.
NAME_IDENTITY = "tripbot"

# Small but explicit requests for the helper containers (migrate init, one-shot
# Job containers). Namespaces under a ResourceQuota that enforces requests.*
# (stage-1's app-quota) reject any pod whose containers omit requests for the
# quota'd resources — so every container must declare them.
SMALL_RESOURCES = k8s.ResourceRequirements(
    requests={
        "cpu": k8s.Quantity.from_string("100m"),
        "memory": k8s.Quantity.from_string("128Mi"),
    }
)

# Constant base ConfigMap literals (base kustomization configMapGenerator). The
# sibling-service hosts (VLC/ONSCREENS/OBS_SERVER_HOST) are per-platform, so
# they're assembled in config_data() from app_name rather than held as literals.
_BASE_CONFIG = {
    "READ_ONLY": "false",
    "VERBOSE": "false",
    "MAPS_OUTPUT_DIR": "/opt/data/maps",
    "DATABASE_HOST": "postgres",
    "TRIPBOT_SERVER_PORT": "8080",
}


def config_map_name(platform: str) -> str:
    """The per-platform tripbot ConfigMap name (stable, non-hashed) — the
    bootstrap/seed Jobs envFrom it by name. e.g. tripbot-twitch-config."""
    return f"{app_name('tripbot', platform)}-config"


# Placeholder DB creds for the laptop `local` overlay (gitignored secret.env in
# Kustomize). DB-only — everything else comes from ESO even locally.
_LOCAL_SECRET = {
    "DATABASE_USER": "tripbot_docker",
    "DATABASE_PASS": "hunter2",
    "DATABASE_DB": "tripbot_docker",
}

# Per-env tripbot identity + config values the overlays vary. Channel identity:
# prod is the real adanalife_ channel on tripbot4000. ALL non-prod envs
# (dev + stage + local) use tripbot4001 in adanalife_staging — reserving
# tripbot4000 for prod. They share tripbot4001 (all on the tripbot-development
# Twitch app), so any two can't be live at once and a re-auth rotates the shared
# token; that clobbering is accepted as low-stakes off-prod. NATS_URL/
# DISCORD_GUILD_ID/ONSCREENS override are per-env extras layered on the base.
_ENV_CONFIG: dict[str, dict[str, str]] = {
    "prod-1": {
        "CHANNEL_NAME": "adanalife_",
        "BOT_USERNAME": "tripbot4000",
        "GOOGLE_APPS_PROJECT_ID": "tripbot-prod",
    },
    "stage-1": {
        "CHANNEL_NAME": "adanalife_staging",
        # tripbot4001 (the non-prod test bot), not the prod tripbot4000.
        "BOT_USERNAME": "tripbot4001",
        "GOOGLE_APPS_PROJECT_ID": "tripbot-stage",
        # ADanaLife guild snowflake — stage's discord bot is gated to it.
        "DISCORD_GUILD_ID": "607964164220125258",
    },
    "development": {
        "CHANNEL_NAME": "adanalife_staging",
        # tripbot4001 (shared with stage), not the prod tripbot4000.
        "BOT_USERNAME": "tripbot4001",
        "GOOGLE_APPS_PROJECT_ID": "tripbot-stage",
    },
    "local": {
        "CHANNEL_NAME": "adanalife_staging",
        # tripbot4001 (the non-prod test bot), not the prod tripbot4000.
        "BOT_USERNAME": "tripbot4001",
        "GOOGLE_APPS_PROJECT_ID": "tripbot-stage",
    },
}


def public_host(env: EnvConfig, platform: str) -> str:
    """The instance's public host: per-name everywhere (tripbot-twitch.<dns>,
    tripbot-youtube.<dns>); the .localhost TLD when the env publishes no DNS.
    Single source for the Ingress rule, external-dns annotation, TLS secret
    host, and EXTERNAL_URL — they can't drift apart."""
    name = app_name("tripbot", platform)
    return f"{name}.localhost" if not env.dns_base else f"{name}.{env.dns_base}"


def external_url(env: EnvConfig, platform: str) -> str:
    """EXTERNAL_URL for an instance: scheme + public_host (+ the env's
    non-standard port, e.g. dev's :9443). Both OAuth flows build their redirect
    as EXTERNAL_URL + /auth/callback, so this must match the host the instance
    actually serves on — and each value needs a matching authorized redirect
    URI registered on the platform's OAuth app (Twitch dev console / GCP
    console)."""
    scheme = "https" if env.dns_base else "http"
    port = f":{env.external_port}" if env.external_port else ""
    return f"{scheme}://{public_host(env, platform)}{port}"


def config_data(env: EnvConfig, platform: str) -> dict[str, str]:
    """The assembled tripbot-config data for an env+platform: base literals + the
    per-platform sibling-service hosts (this platform's vlc/onscreens/obs) +
    telemetry + the per-env identity/extra block. Shared with the Jobs so a Job
    applied on its own carries the same config the Deployment runs with. NATS_URL
    is only present where the env defines one (absent on local)."""
    data = dict(_BASE_CONFIG)
    # bare "postgres" when co-located (parity); cross-namespace FQDN when the DB
    # is isolated in its own namespace (env.data_namespace).
    data["DATABASE_HOST"] = env.postgres_host
    data["VLC_SERVER_HOST"] = f"{app_name('vlc', platform)}:8080"
    data["ONSCREENS_SERVER_HOST"] = f"{app_name('onscreens', platform)}:8080"
    data["OBS_SERVER_HOST"] = f"{app_name('obs', platform)}:8080"
    # OBS WebSocket control addr (port 4455) — distinct from OBS_SERVER_HOST's
    # :8080 Flask health server. Read directly by tripbot's pkg/obs (watchdog +
    # stream start/stop); must be per-platform so the YouTube stack dials
    # obs-youtube, not obs-twitch. Was previously unset, leaving pkg/obs on its
    # stale "obs:4455" default after OBS went per-platform.
    data["OBS_WEBSOCKET_ADDR"] = f"{app_name('obs', platform)}:4455"
    # tripbot's Run() branches on STREAM_PLATFORM (chat transport, command
    # allowlist, Twitch-only boot steps). twitch is the binary's default, so —
    # same idiom as the OBS chart — only non-twitch instances carry the key,
    # keeping the long-running twitch ConfigMaps (and their config-hash
    # rollouts) untouched.
    if platform != "twitch":
        data["STREAM_PLATFORM"] = platform
    data.update(appconfig.telemetry_config(env, platform))
    data.update(_ENV_CONFIG[env.name])
    data["EXTERNAL_URL"] = external_url(env, platform)
    if env.nats_url:
        data["NATS_URL"] = env.nats_url
    return data


class Tripbot(Construct):
    def __init__(self, scope: Construct, platform: str, *, env: EnvConfig):
        name = app_name("tripbot", platform)  # tripbot-twitch / tripbot-youtube
        super().__init__(scope, name)
        ns = env.namespace or None
        labels = meta_labels(name)
        sel = selector(name)
        image = f"{IMAGE}:{env.tag_for('tripbot')}"
        pull = env.pull_policy_for("tripbot")
        cm_name = config_map_name(platform)
        local = env.secret_source == "local"
        db_secret = LOCAL_DB_SECRET if local else DB_SECRET_NAME

        # --- ConfigMap (per-platform stable name + content-hash annotation) ---
        data = config_data(env, platform)
        cfg_hash = configmap.config_map(
            self, "config", name=cm_name, namespace=ns, labels=labels, data=data
        )

        # tripbot's DB + app Secrets are identity-level (one bot, one DB — shared by
        # every platform stack in the namespace), so they're emitted ONCE in the
        # per-env supporting unit (emit_identity_secrets), not here. The component
        # just envFroms them by name below.

        # --- envFrom: config, DB creds, shared OTLP/Sentry, then app Secrets ---
        # Order matches the legacy render exactly (later entries win on key
        # collision). The two discord Secrets are optional so the bot boots
        # without them; everything else is required (a missing Secret fails loud).
        env_from = [
            k8s.EnvFromSource(config_map_ref=k8s.ConfigMapEnvSource(name=cm_name)),
            k8s.EnvFromSource(secret_ref=k8s.SecretEnvSource(name=db_secret)),
            k8s.EnvFromSource(
                secret_ref=k8s.SecretEnvSource(name="grafana-cloud-otlp")
            ),
            k8s.EnvFromSource(secret_ref=k8s.SecretEnvSource(name="sentry-tripbot")),
            k8s.EnvFromSource(
                secret_ref=k8s.SecretEnvSource(name="tripbot-twitch-creds")
            ),
            k8s.EnvFromSource(
                secret_ref=k8s.SecretEnvSource(name="tripbot-google-maps-api-key")
            ),
            k8s.EnvFromSource(
                secret_ref=k8s.SecretEnvSource(
                    name="tripbot-discord-alerts-webhook", optional=True
                )
            ),
            k8s.EnvFromSource(
                secret_ref=k8s.SecretEnvSource(
                    name="tripbot-discord-bot-token", optional=True
                )
            ),
        ]

        # YouTube OAuth client creds are per-platform (only the youtube instance
        # reads them), so unlike the identity-level Secrets above the
        # ExternalSecret is emitted HERE, with the instance — the whole footprint
        # appears/disappears with the platform's entry in env.platforms. The SM
        # JSON holds YOUTUBE_CLIENT_ID + YOUTUBE_CLIENT_SECRET (+ optionally
        # YOUTUBE_CHANNEL_ID, the prod identity pin); extract mode materializes
        # whichever keys exist.
        if platform == "youtube":
            eso.external_secret(
                self,
                "youtube-creds-external-secret",
                name="tripbot-youtube-creds",
                namespace=ns,
                labels=labels,
                creation_policy="Owner",
                extract="k8s/tripbot/youtube-creds",
            )
            env_from.append(
                k8s.EnvFromSource(
                    secret_ref=k8s.SecretEnvSource(name="tripbot-youtube-creds")
                )
            )

        hardened = k8s.SecurityContext(
            allow_privilege_escalation=False,
            capabilities=k8s.Capabilities(drop=["ALL"]),
        )

        # migrate initContainer: `migrate up` to head before tripbot starts.
        # Idempotent (a no-op when already at head); on a fresh cluster it
        # populates the schema before the main container's LoadFromDB runs.
        migrate = k8s.Container(
            name="migrate",
            image=image,
            image_pull_policy=pull,
            security_context=hardened,
            command=["migrate"],
            args=[
                "-path",
                "/migrations",
                "-database",
                "postgres://$(DATABASE_USER):$(DATABASE_PASS)@$(DATABASE_HOST):5432/$(DATABASE_DB)?sslmode=disable",
                "up",
            ],
            env_from=[
                k8s.EnvFromSource(config_map_ref=k8s.ConfigMapEnvSource(name=cm_name)),
                k8s.EnvFromSource(secret_ref=k8s.SecretEnvSource(name=db_secret)),
            ],
            resources=SMALL_RESOURCES,
        )

        container = k8s.Container(
            name=name,
            image=image,
            image_pull_policy=pull,
            security_context=hardened,
            ports=[k8s.ContainerPort(name="http", container_port=8080)],
            # USER must be set so OTel's process resource detector (user.Current)
            # doesn't crash telemetry init on a no-/etc/passwd uid-65532 binary.
            env=[k8s.EnvVar(name="USER", value="tripbot")],
            env_from=env_from,
            liveness_probe=k8s.Probe(
                http_get=k8s.HttpGetAction(
                    path="/health/live", port=k8s.IntOrString.from_string("http")
                ),
                initial_delay_seconds=15,
                period_seconds=30,
                timeout_seconds=5,
            ),
            readiness_probe=k8s.Probe(
                http_get=k8s.HttpGetAction(
                    path="/health/ready", port=k8s.IntOrString.from_string("http")
                ),
                initial_delay_seconds=5,
                period_seconds=10,
            ),
            resources=k8s.ResourceRequirements(
                requests={
                    "cpu": k8s.Quantity.from_string("100m"),
                    "memory": k8s.Quantity.from_string("256Mi"),
                },
                limits={"memory": k8s.Quantity.from_string("1Gi")},
            ),
        )

        k8s.KubeDeployment(
            self,
            "deployment",
            metadata=k8s.ObjectMeta(name=name, namespace=ns, labels=labels),
            spec=k8s.DeploymentSpec(
                replicas=1,
                selector=k8s.LabelSelector(match_labels=sel),
                template=k8s.PodTemplateSpec(
                    metadata=k8s.ObjectMeta(
                        labels=sel, annotations=configmap.pod_annotations(cfg_hash)
                    ),
                    spec=k8s.PodSpec(
                        security_context=k8s.PodSecurityContext(
                            run_as_non_root=True,
                            run_as_user=65532,
                            run_as_group=65532,
                            seccomp_profile=k8s.SeccompProfile(type="RuntimeDefault"),
                        ),
                        priority_class_name=env.priority_class or None,
                        init_containers=[migrate],
                        containers=[container],
                    ),
                ),
            ),
        )

        # --- Service ---
        k8s.KubeService(
            self,
            "service",
            metadata=k8s.ObjectMeta(name=name, namespace=ns, labels=labels),
            spec=k8s.ServiceSpec(
                type="ClusterIP",
                selector=sel,
                ports=[
                    k8s.ServicePort(
                        name="http",
                        port=8080,
                        target_port=k8s.IntOrString.from_string("http"),
                    )
                ],
            ),
        )

        # --- Ingress (dashboard / OAuth) — published in every env ---
        self._ingress(name, platform, env, ns, labels)
        if env.tailscale:
            self._tailscale_ingress(name, env, ns, labels)

    # ---- Ingress helpers ----
    def _ingress(self, name, platform, env: EnvConfig, ns, labels):
        # Per-name host shared with EXTERNAL_URL via public_host() — symmetric
        # with the other per-platform components. local uses the .localhost TLD
        # (no DNS/TLS); every other env publishes a real host with external-dns
        # + cert-manager TLS (DNS-01 Route53).
        host = public_host(env, platform)
        ann = (
            {}
            if not env.dns_base
            else {
                "external-dns.alpha.kubernetes.io/hostname": host,
                "cert-manager.io/issuer": "letsencrypt-route53",
            }
        )
        tls = bool(env.dns_base)  # every DNS-publishing env issues a cert
        backend = k8s.IngressBackend(
            service=k8s.IngressServiceBackend(
                name=name, port=k8s.ServiceBackendPort(name="http")
            )
        )
        k8s.KubeIngress(
            self,
            "ingress",
            metadata=k8s.ObjectMeta(
                name=name, namespace=ns, labels=labels, annotations=ann or None
            ),
            spec=k8s.IngressSpec(
                ingress_class_name="traefik",
                tls=[k8s.IngressTls(hosts=[host], secret_name=f"{name}-tls")]
                if tls
                else None,
                rules=[
                    k8s.IngressRule(
                        host=host,
                        http=k8s.HttpIngressRuleValue(
                            paths=[
                                k8s.HttpIngressPath(
                                    path="/", path_type="Prefix", backend=backend
                                )
                            ]
                        ),
                    )
                ],
            ),
        )

    def _tailscale_ingress(self, name, env: EnvConfig, ns, labels):
        short = env.dns_base.split(".")[0]  # prod / stage
        k8s.KubeIngress(
            self,
            "ts-ingress",
            metadata=k8s.ObjectMeta(name=f"{name}-ts", namespace=ns),
            spec=k8s.IngressSpec(
                ingress_class_name="tailscale",
                default_backend=k8s.IngressBackend(
                    service=k8s.IngressServiceBackend(
                        name=name, port=k8s.ServiceBackendPort(number=8080)
                    )
                ),
                tls=[k8s.IngressTls(hosts=[f"{name}-{short}"])],
            ),
        )


# ---------------------------------------------------------------------------
# Identity-level Secrets — emitted ONCE per env in the supporting unit, not per
# component. tripbot is one bot identity against one DB, shared by every platform
# stack in the namespace, so its DB creds + app Secrets are namespace
# infrastructure rather than a per-component concern. Names are unchanged.
# ---------------------------------------------------------------------------


def emit_identity_secrets(scope: Construct, env: EnvConfig) -> None:
    """tripbot's DB creds (ESO ExternalSecret on eso envs, on-disk Secret on the
    laptop) + the twitch/maps/discord ExternalSecrets. Called by SupportingChart."""
    ns = env.namespace or None
    labels = meta_labels(NAME_IDENTITY)
    if env.secret_source == "local":
        k8s.KubeSecret(
            scope,
            "secret",
            metadata=k8s.ObjectMeta(name=LOCAL_DB_SECRET, namespace=ns),
            type="Opaque",
            string_data=dict(_LOCAL_SECRET),
        )
    else:
        _emit_db_external_secret(scope, ns, labels)
    _emit_app_external_secrets(scope, ns, labels)


def _emit_db_external_secret(scope, ns, labels):
    # database creds: reads the shared postgres SM JSON ({user,password,db})
    # and remaps it onto DATABASE_* keys via target.template — a shape the
    # eso.external_secret helper doesn't cover, so emit it as a raw ApiObject
    # (same idiom as obs.py / postgres.py).
    meta: dict = {"name": DB_SECRET_NAME}
    if ns:
        meta["namespace"] = ns
    if labels:
        meta["labels"] = labels
    es = cdk8s.ApiObject(
        scope,
        "database-external-secret",
        api_version="external-secrets.io/v1",
        kind="ExternalSecret",
        metadata=meta,
    )
    es.add_json_patch(
        cdk8s.JsonPatch.add(
            "/spec",
            {
                "refreshInterval": "1h",
                "secretStoreRef": {
                    "name": "aws-secretsmanager",
                    "kind": "SecretStore",
                },
                "target": {
                    "name": DB_SECRET_NAME,
                    "template": {
                        "type": "Opaque",
                        "data": {
                            "DATABASE_USER": "{{ .user }}",
                            "DATABASE_PASS": "{{ .password }}",
                            "DATABASE_DB": "{{ .db }}",
                        },
                    },
                },
                "data": [
                    {
                        "secretKey": "user",
                        "remoteRef": {
                            "key": "k8s/postgres/credentials",
                            "property": "user",
                        },
                    },
                    {
                        "secretKey": "password",
                        "remoteRef": {
                            "key": "k8s/postgres/credentials",
                            "property": "password",
                        },
                    },
                    {
                        "secretKey": "db",
                        "remoteRef": {
                            "key": "k8s/postgres/credentials",
                            "property": "db",
                        },
                    },
                ],
            },
        )
    )


def _emit_app_external_secrets(scope, ns, labels):
    # twitch + google-maps: extract every top-level key of the SM JSON blob.
    for id_, name, sm in [
        (
            "twitch-external-secret",
            "tripbot-twitch-creds",
            "k8s/tripbot/twitch-creds",
        ),
        (
            "google-maps-external-secret",
            "tripbot-google-maps-api-key",
            "k8s/tripbot/google-maps-api-key",
        ),
    ]:
        eso.external_secret(
            scope,
            id_,
            name=name,
            namespace=ns,
            labels=labels,
            creation_policy="Owner",
            extract=sm,
        )
    # discord alerts + bot-token: one SM container → one materialized key.
    for id_, name, sm, key in [
        (
            "discord-alerts-external-secret",
            "tripbot-discord-alerts-webhook",
            "k8s/tripbot/discord-alerts-webhook",
            "DISCORD_ALERTS_WEBHOOK",
        ),
        (
            "discord-bot-token-external-secret",
            "tripbot-discord-bot-token",
            "k8s/tripbot/discord-bot-token",
            "DISCORD_BOT_TOKEN",
        ),
    ]:
        eso.external_secret(
            scope,
            id_,
            name=name,
            namespace=ns,
            labels=labels,
            creation_policy="Owner",
            data=[ESData(key, sm)],
        )


# ---------------------------------------------------------------------------
# One-shot Jobs — module-level emitters, NOT auto-run by the Tripbot construct.
#
# They are deliberately separate so a routine app apply never fires a one-shot.
# The deploy tasks emit + apply
# them on demand. All `envFrom` the PRIMARY platform's tripbot ConfigMap by name
# (config_map_name(env.platforms[0]), e.g. tripbot-twitch-config) — the Jobs are
# identity-level (one bot, one DB), not per-platform. On the
# laptop the DB Secret is the secret.env-built `tripbot-secret` and the bootstrap
# is a single combined Job; on eso envs the DB Secret is `tripbot-database-creds`
# and the bootstrap splits into bot + broadcaster legs.
# ---------------------------------------------------------------------------


def emit_jobs(scope: Construct, env: EnvConfig) -> None:
    """Emit the env's one-shot Jobs. local gets the combined auth-bootstrap +
    seed (secret.env-wired); eso envs get the bot + broadcaster auth-bootstrap
    legs + the ESO-wired seed."""
    if env.secret_source == "local":
        local_auth_bootstrap(scope, env)
        seed(scope, env)
    else:
        auth_bootstrap_bot(scope, env)
        auth_bootstrap_broadcaster(scope, env)
        seed(scope, env)


def _auth_bootstrap(
    scope: Construct,
    id: str,
    name: str,
    env: EnvConfig,
    *,
    account: str | None,
    db_secret: str,
    twitch_optional: bool,
) -> None:
    ns = env.namespace or None
    args = [f"--account={account}"] if account else None
    env_from = [
        k8s.EnvFromSource(
            config_map_ref=k8s.ConfigMapEnvSource(
                name=config_map_name(env.platforms[0])
            )
        ),
        k8s.EnvFromSource(secret_ref=k8s.SecretEnvSource(name=db_secret)),
        k8s.EnvFromSource(
            secret_ref=k8s.SecretEnvSource(
                name="tripbot-twitch-creds", optional=twitch_optional
            )
        ),
    ]
    # The eso bootstrap legs also pull maps creds; the local combined Job
    # (account=None) stops at twitch (matches overlays/local/auth-job.yaml).
    if account is not None:
        env_from.append(
            k8s.EnvFromSource(
                secret_ref=k8s.SecretEnvSource(
                    name="tripbot-google-maps-api-key", optional=False
                )
            )
        )
    container = k8s.Container(
        name="auth-bootstrap",
        image=f"{IMAGE}:{env.tag_for('tripbot')}",
        image_pull_policy=env.pull_policy_for("tripbot"),
        command=["/usr/local/bin/auth-bootstrap"],
        args=args,
        ports=[k8s.ContainerPort(container_port=8080)],
        # The OAuth callback must return to this port-forwarded pod, so
        # EXTERNAL_URL is pinned to localhost:8080 (overrides the config host).
        env=[k8s.EnvVar(name="EXTERNAL_URL", value="http://localhost:8080")],
        env_from=env_from,
        resources=SMALL_RESOURCES,
    )
    k8s.KubeJob(
        scope,
        id,
        metadata=k8s.ObjectMeta(name=name, namespace=ns),
        spec=k8s.JobSpec(
            backoff_limit=0,
            ttl_seconds_after_finished=600,
            template=k8s.PodTemplateSpec(
                spec=k8s.PodSpec(restart_policy="Never", containers=[container])
            ),
        ),
    )


def auth_bootstrap_bot(scope: Construct, env: EnvConfig) -> None:
    """ESO-wired bot-account auth-bootstrap (credentialed envs)."""
    _auth_bootstrap(
        scope,
        "auth-bootstrap-bot",
        "tripbot-auth-bootstrap-bot",
        env,
        account="bot",
        db_secret=DB_SECRET_NAME,
        twitch_optional=False,
    )


def auth_bootstrap_broadcaster(scope: Construct, env: EnvConfig) -> None:
    """ESO-wired broadcaster-account auth-bootstrap (credentialed envs)."""
    _auth_bootstrap(
        scope,
        "auth-bootstrap-broadcaster",
        "tripbot-auth-bootstrap-broadcaster",
        env,
        account="broadcaster",
        db_secret=DB_SECRET_NAME,
        twitch_optional=False,
    )


def local_auth_bootstrap(scope: Construct, env: EnvConfig) -> None:
    """Laptop combined auth-bootstrap (secret.env-wired, no --account split)."""
    _auth_bootstrap(
        scope,
        "auth-bootstrap",
        "tripbot-auth-bootstrap",
        env,
        account=None,
        db_secret=LOCAL_DB_SECRET,
        twitch_optional=True,
    )


def seed(scope: Construct, env: EnvConfig) -> None:
    """DB seed Job: wait-for-postgres → migrate → seed-db. envFroms the stable
    tripbot-config + the env's DB Secret (secret.env on local, ESO elsewhere).
    local's variant has backoffLimit 3 and no ttl (matches seed-job.yaml)."""
    ns = env.namespace or None
    local = env.secret_source == "local"
    db_secret = LOCAL_DB_SECRET if local else DB_SECRET_NAME
    image = f"{IMAGE}:{env.tag_for('tripbot')}"
    pull = env.pull_policy_for("tripbot")
    db_env = [
        k8s.EnvFromSource(
            config_map_ref=k8s.ConfigMapEnvSource(
                name=config_map_name(env.platforms[0])
            )
        ),
        k8s.EnvFromSource(secret_ref=k8s.SecretEnvSource(name=db_secret)),
    ]

    wait = k8s.Container(
        name="wait-for-postgres",
        image="busybox:1.36",
        command=[
            "sh",
            "-c",
            f'until nc -z {env.postgres_host} 5432; do echo "waiting for postgres..."; sleep 2; done',
        ],
        resources=SMALL_RESOURCES,
    )
    migrate = k8s.Container(
        name="migrate",
        image=image,
        image_pull_policy=pull,
        command=["migrate"],
        args=[
            "-path",
            "/migrations",
            "-database",
            "postgres://$(DATABASE_USER):$(DATABASE_PASS)@$(DATABASE_HOST):5432/$(DATABASE_DB)?sslmode=disable",
            "up",
        ],
        env_from=db_env,
        resources=SMALL_RESOURCES,
    )
    seed_c = k8s.Container(
        name="seed",
        image=image,
        image_pull_policy=pull,
        command=["/usr/local/bin/seed-db"],
        env_from=db_env,
        resources=SMALL_RESOURCES,
    )

    spec = k8s.JobSpec(
        backoff_limit=3,
        # eso seed cleans up after itself; the laptop variant lingers (no ttl).
        ttl_seconds_after_finished=None if local else 600,
        template=k8s.PodTemplateSpec(
            spec=k8s.PodSpec(
                restart_policy="Never",
                init_containers=[wait, migrate],
                containers=[seed_c],
            )
        ),
    )
    k8s.KubeJob(
        scope,
        "seed",
        metadata=k8s.ObjectMeta(name="tripbot-seed", namespace=ns),
        spec=spec,
    )
