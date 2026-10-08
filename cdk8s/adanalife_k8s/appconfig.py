"""Config-literal blocks shared by the Go services (tripbot + onscreens-server
share config surface). Kept here so the constructs assemble identical
telemetry/stub blocks instead of drifting.
"""

from __future__ import annotations

from adanalife_k8s.config import EnvConfig


def telemetry_config(env: EnvConfig, platform: str) -> dict[str, str]:
    """ENV + OTEL_* + SENTRY_ENVIRONMENT — the per-env telemetry block every
    Go service (tripbot/onscreens-server) merges onto its base
    ConfigMap. `platform` is stamped into the OTel resource attributes as
    `service.platform`, which Grafana Cloud surfaces as a `service_platform`
    metric/log label so dashboards can filter twitch vs youtube."""
    return {
        "ENV": env.binary_env,
        "OTEL_SDK_DISABLED": env.otel_disabled,
        # The in-cluster Alloy OTLP collector, which fans metrics out to
        # VictoriaMetrics and forwards metrics/logs/traces to Grafana Cloud —
        # it holds the cloud credential, so the pods need no auth header.
        # Base URL only: the SDK appends /v1/metrics, /v1/traces, /v1/logs.
        "OTEL_EXPORTER_OTLP_ENDPOINT": "http://k8s-monitoring-alloy-receiver.monitoring.svc:4318",
        "OTEL_TRACES_SAMPLER": "parentbased_traceidratio",
        "OTEL_TRACES_SAMPLER_ARG": "0.1",
        "OTEL_RESOURCE_ATTRIBUTES": f"deployment.environment={env.deployment_env},service.namespace=tripbot,service.platform={platform}",
        "SENTRY_ENVIRONMENT": env.sentry_env,
    }
