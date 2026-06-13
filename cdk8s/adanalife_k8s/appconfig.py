"""Config-literal blocks shared by the Go services (vlc-server + tripbot share
one config package, so they share env-var surface). Kept here so the two
constructs assemble identical telemetry/stub blocks instead of drifting.
"""

from __future__ import annotations

from adanalife_k8s.config import EnvConfig


def telemetry_config(env: EnvConfig, platform: str) -> dict[str, str]:
    """ENV + OTEL_* + SENTRY_ENVIRONMENT — the per-env telemetry block every
    Go service (tripbot/vlc-server/onscreens-server) merges onto its base
    ConfigMap. `platform` is stamped into the OTel resource attributes as
    `service.platform`, which Grafana Cloud surfaces as a `service_platform`
    metric/log label so dashboards can filter twitch vs youtube."""
    return {
        "ENV": env.binary_env,
        "OTEL_SDK_DISABLED": env.otel_disabled,
        "OTEL_TRACES_SAMPLER": "parentbased_traceidratio",
        "OTEL_TRACES_SAMPLER_ARG": "0.1",
        "OTEL_RESOURCE_ATTRIBUTES": f"deployment.environment={env.deployment_env},service.namespace=tripbot,service.platform={platform}",
        "SENTRY_ENVIRONMENT": env.sentry_env,
    }


def local_stubs() -> dict[str, str]:
    """DB / Twitch stub values the local overlay injects (and that the
    development overlay inherits by extending local). Absent on stage/prod,
    where the real values arrive via ESO Secrets."""
    return {
        "DATABASE_HOST": "postgres",
        "DATABASE_USER": "tripbot_docker",
        "DATABASE_PASS": "hunter2",
        "DATABASE_DB": "tripbot_docker",
        "CHANNEL_NAME": "adanalife",
        "BOT_USERNAME": "adanalife_bot",
        "TWITCH_CLIENT_ID": "stub",
        "TWITCH_CLIENT_SECRET": "stub",
        "TWITCH_AUTH_TOKEN": "oauth:stub",
    }


def uses_local_stubs(env: EnvConfig) -> bool:
    """Local + development carry the stub block (development extends local)."""
    return env.name in ("local", "development")
