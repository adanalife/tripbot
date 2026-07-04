"""Deploy units for the tripbot app workloads. Each Chart synthesizes to one file
in dist/ and is applied independently — one per (component, platform) for the apps
(emit_app_charts), one per env for the identity Secrets + stream protection
(IdentityChart), plus the one-shot Job charts (emit_job_charts).

This is the subset of infra/cdk8s's charts.py that moved to the tripbot repo: the
three images built from this repo (tripbot, vlc, onscreens) and tripbot's
identity Secrets. OBS is built + deployed from the standalone adanalife/obs repo
(its own cdk8s). The STATEFUL + shared-platform units stay in infra/cdk8s:
postgres (DataChart), the ESO SecretStore, the shared observability Secrets +
cert-manager Issuers (SupportingChart), the dashcam PV/PVC, and the Argo config.
Apps reference the materialized Secret names emitted by those infra units by name
(grafana-cloud-otlp / sentry-* / the DB creds) — that naming is the contract
between the two repos.
"""

from __future__ import annotations

from cdk8s import Chart
from constructs import Construct

from adanalife_k8s.config import EnvConfig
from adanalife_k8s.constructs.onscreens import OnscreensServer
from adanalife_k8s.constructs.tripbot import Tripbot, emit_identity_secrets
from adanalife_k8s.constructs.vlc import VlcServer
from adanalife_k8s.stream_protection import emit_stream_protection

# Stateless app components that each get their own Chart (→ one dist file + one
# Argo Application) per (env, platform). OBS is built + deployed from the
# standalone adanalife/obs repo, not here. Keep this list in sync with the
# contract's per-platform service keys; naming.app_name maps (component,
# platform) -> the Service name.
COMPONENTS = ("tripbot", "vlc", "onscreens")
_SIMPLE_COMPONENTS = (
    ("tripbot", Tripbot),
    ("vlc", VlcServer),
    ("onscreens", OnscreensServer),
)


def emit_app_charts(scope: Construct, env: EnvConfig) -> None:
    """One Chart per (component, platform) — each synthesizes to its own
    `dist/<env>-<component>-<platform>.k8s.yaml`, so every component is an
    independent Argo Application (one sync/health/URL). The identity + one-shot
    units stay separate (IdentityChart / emit_job_charts).
    """
    ns = env.namespace or None
    for platform in env.platforms:
        for comp, ctor in _SIMPLE_COMPONENTS:
            chart = Chart(scope, f"{env.name}-{comp}-{platform}", namespace=ns)
            ctor(chart, platform, env=env)


class IdentityChart(Chart):
    """tripbot's per-env identity-level Secrets (DB creds + twitch/maps/discord —
    one bot identity, one DB, shared by every platform stack in the namespace),
    plus the prod-stream PriorityClass + ResourceQuota. Synthesizes to
    `dist/<env>-tripbot-identity.k8s.yaml` — its own deploy unit / Argo Application,
    isolated from the per-component app churn so the DB-creds ExternalSecret isn't
    re-applied on every app sync.

    Depends on infra's ESO SecretStore (the `aws-parameterstore` store these
    ExternalSecrets reference) + the shared observability Secrets being present
    first — same data→supporting→apps ordering as before, now spanning two repos.
    """

    def __init__(self, scope: Construct, id: str, *, env: EnvConfig):
        super().__init__(scope, id, namespace=env.namespace or None)
        self.env = env
        # tripbot identity Secrets (every env — on-disk Secret on the laptop, ESO
        # ExternalSecrets on credentialed envs).
        emit_identity_secrets(self, env)
        # prod-stream PriorityClass + co-tenant ResourceQuota (knob-gated).
        emit_stream_protection(self, env)


def emit_job_charts(scope: Construct, env: EnvConfig) -> None:
    """tripbot one-shot Jobs — one Chart each, so every Job synthesizes to its own
    `dist/<env>-job-<name>.k8s.yaml` and a deploy task can `kubectl apply` exactly
    one. NOT auto-run on a normal apply (running a seed/auth Job on every reconcile
    would be wrong) — invoked via `task tripbot:<env>:{db:seed,auth:bootstrap:*}`.
    local gets the combined auth-bootstrap; eso envs get the bot + broadcaster
    legs; both get seed."""
    from adanalife_k8s.constructs import tripbot as tb

    ns = env.namespace or None

    def _chart(suffix: str) -> Chart:
        return Chart(scope, f"{env.name}-job-{suffix}", namespace=ns)

    if env.secret_source == "local":
        tb.local_auth_bootstrap(_chart("auth-bootstrap"), env)
    else:
        tb.auth_bootstrap_bot(_chart("auth-bootstrap-bot"), env)
        tb.auth_bootstrap_broadcaster(_chart("auth-bootstrap-broadcaster"), env)
    tb.seed(_chart("seed"), env)
